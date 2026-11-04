package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/golang-cafe/job-board/internal/user"
	"github.com/gorilla/mux"
	"github.com/segmentio/ksuid"
)

const maxDeveloperCVSize = 5 * 1024 * 1024

var safeSlugRe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type devCVRepository interface {
	DeveloperProfileByID(id string) (developer.Developer, error)
	SaveDeveloperCV(id string, cv []byte) error
	GetDeveloperCVBySlug(slug string) (developer.Developer, error)
	TrackDeveloperCVDownload(id, developerProfileID string, cv []byte, userID string) error
}

func validateDeveloperCV(cvBytes []byte) error {
	if len(cvBytes) == 0 {
		return errors.New("cv file is empty")
	}
	if len(cvBytes) > maxDeveloperCVSize {
		return errors.New("cv file size must not exceed 5MB")
	}
	if !bytes.HasPrefix(cvBytes, []byte("%PDF-")) || http.DetectContentType(cvBytes) != "application/pdf" {
		return errors.New("cv must be a valid PDF file")
	}
	return nil
}

func decodeAndValidateBase64CV(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if idx := strings.Index(trimmed, ","); idx != -1 && strings.HasPrefix(strings.ToLower(trimmed[:idx]), "data:") {
		trimmed = trimmed[idx+1:]
	}
	cvBytes, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		cvBytes, err = base64.RawStdEncoding.DecodeString(trimmed)
		if err != nil {
			return nil, errors.New("invalid base64 cv payload")
		}
	}
	if err := validateDeveloperCV(cvBytes); err != nil {
		return nil, err
	}
	return cvBytes, nil
}

func parseMultipartDeveloperCV(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxDeveloperCVSize+1024*1024)
	if err := r.ParseMultipartForm(maxDeveloperCVSize); err != nil {
		return nil, errors.New("unable to parse multipart form or file exceeds 5MB limit")
	}
	file, _, err := r.FormFile("cv")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return nil, nil
		}
		return nil, errors.New("unable to read cv file from form")
	}
	defer file.Close()

	cvBytes, err := io.ReadAll(io.LimitReader(file, maxDeveloperCVSize+1))
	if err != nil {
		return nil, errors.New("unable to read cv file bytes")
	}
	if err := validateDeveloperCV(cvBytes); err != nil {
		return nil, err
	}
	return cvBytes, nil
}

func UploadDeveloperCVHandler(svr server.Server, devRepo devCVRepository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get user from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}

			var developerProfileID string
			var cvBytes []byte

			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				parsedCV, err := parseMultipartDeveloperCV(w, r)
				if err != nil {
					svr.JSON(w, http.StatusBadRequest, err.Error())
					return
				}
				cvBytes = parsedCV
				developerProfileID = strings.TrimSpace(r.FormValue("id"))
			} else {
				req := &struct {
					ID string `json:"id"`
					CV string `json:"cv"`
				}{}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					svr.JSON(w, http.StatusBadRequest, "invalid request payload")
					return
				}
				developerProfileID = strings.TrimSpace(req.ID)
				if strings.TrimSpace(req.CV) != "" {
					decodedCV, err := decodeAndValidateBase64CV(req.CV)
					if err != nil {
						svr.JSON(w, http.StatusBadRequest, err.Error())
						return
					}
					cvBytes = decodedCV
				}
			}

			if developerProfileID == "" || len(cvBytes) == 0 {
				svr.JSON(w, http.StatusBadRequest, "id and valid PDF cv are required")
				return
			}

			dev, err := devRepo.DeveloperProfileByID(developerProfileID)
			if err != nil {
				svr.Log(err, "unable to find developer profile by id")
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			if !strings.EqualFold(dev.Email, profile.Email) && !profile.IsAdmin {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}

			if err := devRepo.SaveDeveloperCV(dev.ID, cvBytes); err != nil {
				svr.Log(err, "unable to save developer cv")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}

			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func DownloadDeveloperCVHandler(svr server.Server, devRepo devCVRepository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			profileSlug := strings.TrimSpace(vars["slug"])
			if profileSlug == "" {
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}

			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil || profile == nil {
				svr.Log(err, "unable to get user from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}

			dev, err := devRepo.GetDeveloperCVBySlug(profileSlug)
			if err != nil || dev.ID == "" {
				svr.Log(err, "unable to retrieve developer cv by slug")
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}

			isRecruiter := profile.IsRecruiter || profile.Type == user.UserTypeRecruiter
			isAdmin := profile.IsAdmin || profile.Type == user.UserTypeAdmin
			isOwner := strings.EqualFold(profile.Email, dev.Email)
			if !isRecruiter && !isAdmin && !isOwner {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}

			if len(dev.CV) == 0 {
				svr.JSON(w, http.StatusNotFound, "cv not uploaded")
				return
			}

			downloadID, err := ksuid.NewRandom()
			if err != nil {
				svr.Log(err, "unable to generate cv download id")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}

			if err := devRepo.TrackDeveloperCVDownload(downloadID.String(), dev.ID, dev.CV, profile.UserID); err != nil {
				svr.Log(err, "unable to track developer cv download")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}

			safeFilename := safeSlugRe.ReplaceAllString(dev.Slug, "-")
			if safeFilename == "" {
				safeFilename = "developer"
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=\"%s-cv.pdf\"", safeFilename))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(dev.CV)
		},
	)
}
