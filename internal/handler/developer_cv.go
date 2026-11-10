package handler

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/gorilla/mux"
	"github.com/segmentio/ksuid"
)

const MaxDeveloperCVSize = 5 * 1024 * 1024

type developerCVRepository interface {
	DeveloperProfileByID(id string) (developer.Developer, error)
	SaveDeveloperCV(profileID string, cv []byte) error
	GetDeveloperCVBySlug(slug string) (developer.Developer, error)
	TrackDeveloperCVDownload(download developer.DeveloperCVDownload) error
}

func validateDeveloperCVBytes(cvBytes []byte) error {
	if len(cvBytes) == 0 {
		return errors.New("CV file is empty")
	}
	if len(cvBytes) > MaxDeveloperCVSize {
		return errors.New("PDF max size is 5MB")
	}
	if len(cvBytes) < 5 || !bytes.HasPrefix(cvBytes, []byte("%PDF-")) {
		return errors.New("only PDF files are allowed for CV")
	}
	contentType := http.DetectContentType(cvBytes)
	if contentType != "application/pdf" {
		return errors.New("only PDF files are allowed for CV")
	}
	sniffLen := len(cvBytes)
	if sniffLen > 1024 {
		sniffLen = 1024
	}
	lowerHeader := bytes.ToLower(cvBytes[:sniffLen])
	for _, forbidden := range [][]byte{[]byte("<script"), []byte("<html"), []byte("<iframe"), []byte("javascript:")} {
		if bytes.Contains(lowerHeader, forbidden) {
			return errors.New("invalid PDF content")
		}
	}
	lowerAll := bytes.ToLower(cvBytes)
	for _, forbiddenPDFObj := range [][]byte{[]byte("/javascript"), []byte("/js("), []byte("/js "), []byte("/js<"), []byte("/launch")} {
		if bytes.Contains(lowerAll, forbiddenPDFObj) {
			return errors.New("PDF files with embedded active scripts or launch actions are not allowed")
		}
	}
	return nil
}

func decodeAndValidateBase64CV(encoded string) ([]byte, error) {
	trimmed := strings.TrimSpace(encoded)
	if trimmed == "" {
		return nil, nil
	}
	if idx := strings.Index(trimmed, ","); idx != -1 && strings.HasPrefix(trimmed[:idx], "data:") {
		meta := strings.ToLower(trimmed[:idx])
		if !strings.Contains(meta, "application/pdf") {
			return nil, errors.New("only PDF files are allowed for CV")
		}
		trimmed = trimmed[idx+1:]
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, errors.New("invalid base64 CV payload")
	}
	if err := validateDeveloperCVBytes(decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func parseMultipartDeveloperCV(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(MaxDeveloperCVSize+1024*1024))
	if err := r.ParseMultipartForm(int64(MaxDeveloperCVSize)); err != nil {
		return nil, errors.New("unable to parse multipart form or file exceeds 5MB limit")
	}
	file, header, err := r.FormFile("cv")
	if err == http.ErrMissingFile {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("unable to read CV file")
	}
	defer file.Close()
	if header != nil && header.Size > int64(MaxDeveloperCVSize) {
		return nil, errors.New("PDF max size is 5MB")
	}
	fileBytes, err := io.ReadAll(io.LimitReader(file, int64(MaxDeveloperCVSize)+1))
	if err != nil {
		return nil, errors.New("unable to read CV file content")
	}
	if err := validateDeveloperCVBytes(fileBytes); err != nil {
		return nil, err
	}
	return fileBytes, nil
}

func sanitizePDFFileName(slugStr string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, slugStr)
	safe = strings.Trim(safe, "-")
	if safe == "" {
		return "developer-cv.pdf"
	}
	return fmt.Sprintf("%s-cv.pdf", safe)
}

func UploadDeveloperCVHandler(svr server.Server, devRepo developerCVRepository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil || profile == nil {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			var profileID string
			var cvBytes []byte
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				parsedCV, err := parseMultipartDeveloperCV(w, r)
				if err != nil {
					svr.JSON(w, http.StatusBadRequest, err.Error())
					return
				}
				profileID = strings.TrimSpace(r.FormValue("id"))
				cvBytes = parsedCV
			} else {
				req := &struct {
					ID string `json:"id"`
					CV string `json:"cv"`
				}{}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					svr.JSON(w, http.StatusBadRequest, "invalid request payload")
					return
				}
				decodedCV, err := decodeAndValidateBase64CV(req.CV)
				if err != nil {
					svr.JSON(w, http.StatusBadRequest, err.Error())
					return
				}
				profileID = strings.TrimSpace(req.ID)
				cvBytes = decodedCV
			}
			if profileID == "" || len(cvBytes) == 0 {
				svr.JSON(w, http.StatusBadRequest, "profile id and valid PDF CV are required")
				return
			}
			dev, err := devRepo.DeveloperProfileByID(profileID)
			if err != nil {
				svr.Log(err, "unable to find developer profile for CV upload")
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			if !strings.EqualFold(dev.Email, profile.Email) && !profile.IsAdmin {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			if err := devRepo.SaveDeveloperCV(profileID, cvBytes); err != nil {
				svr.Log(err, "unable to save developer CV")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func DownloadDeveloperCVHandler(svr server.Server, devRepo developerCVRepository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			profileSlug := vars["slug"]
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil || profile == nil {
				svr.JSON(w, http.StatusUnauthorized, nil)
				return
			}
			dev, err := devRepo.GetDeveloperCVBySlug(profileSlug)
			if err == sql.ErrNoRows {
				svr.TEXT(w, http.StatusNotFound, "CV not uploaded")
				return
			}
			if err != nil {
				svr.Log(err, "unable to retrieve developer CV")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			isOwner := strings.EqualFold(profile.Email, dev.Email)
			if !profile.IsRecruiter && !profile.IsAdmin && !isOwner {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			if !dev.HasCV || len(dev.CV) == 0 {
				svr.TEXT(w, http.StatusNotFound, "CV not uploaded")
				return
			}
			downloadID, err := ksuid.NewRandom()
			if err != nil {
				svr.Log(err, "unable to generate CV download ID")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			err = devRepo.TrackDeveloperCVDownload(developer.DeveloperCVDownload{
				ID:                 downloadID.String(),
				DeveloperProfileID: dev.ID,
				CV:                 dev.CV,
				UserID:             profile.UserID,
			})
			if err != nil {
				svr.Log(err, "unable to track developer CV download")
			}
			fileName := sanitizePDFFileName(dev.Slug)
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'")
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", fileName))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(dev.CV)
		},
	)
}
