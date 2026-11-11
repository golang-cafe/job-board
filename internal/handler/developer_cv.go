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
	"regexp"
	"strconv"
	"strings"

	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/gorilla/mux"
	"github.com/segmentio/ksuid"
)

// MaxDeveloperCVSize is the largest accepted CV PDF (5 MB, as specified in
// the bounty issue).
const MaxDeveloperCVSize = 5 * 1024 * 1024

// maxDeveloperCVJSONRequestSize bounds JSON upload requests. A base64-encoded
// 5 MB PDF is ~6.7 MB, so 8 MB leaves room for the surrounding JSON while
// still capping memory use (the multipart path is capped separately with
// http.MaxBytesReader).
const maxDeveloperCVJSONRequestSize = 8 * 1024 * 1024

// maxDeveloperCVEncodedSize bounds a base64 CV payload before decoding it.
const maxDeveloperCVEncodedSize = 8 * 1024 * 1024

var linkedinURLPattern = regexp.MustCompile(`^https:\/\/(?:[a-z]{2,3}\.)?linkedin\.com\/.*$`)

// isValidOptionalLinkedInURL reports whether a LinkedIn URL is acceptable.
// An empty URL is valid: LinkedIn is optional on developer signup and
// profile updates (bounty issue #74). A non-empty URL must match the
// LinkedIn URL pattern used elsewhere in this codebase.
func isValidOptionalLinkedInURL(linkedinURL string) bool {
	if linkedinURL == "" {
		return true
	}
	return linkedinURLPattern.MatchString(linkedinURL)
}

// developerCVRepository is the subset of developer.Repository used by the
// CV handlers, so the handlers can be unit-tested with an in-memory fake.
type developerCVRepository interface {
	DeveloperProfileByID(id string) (developer.Developer, error)
	SaveDeveloperCV(profileID string, cv []byte) error
	DeleteDeveloperCV(profileID string) error
	GetDeveloperCVBySlug(slug string) (developer.Developer, error)
	TrackDeveloperCVDownload(download developer.DeveloperCVDownload) error
	GetDeveloperCVDownloads(profileID string, limit int) ([]developer.DeveloperCVDownloadRecord, error)
	GetDeveloperCVDownloadCount(profileID string) (int, error)
}

func validateDeveloperCVBytes(cvBytes []byte) error {
	if len(cvBytes) == 0 {
		return errors.New("CV file is empty")
	}
	if len(cvBytes) > MaxDeveloperCVSize {
		return errors.New("PDF max size is 5MB")
	}
	if len(cvBytes) < 8 || !bytes.HasPrefix(cvBytes, []byte("%PDF-")) {
		return errors.New("only PDF files are allowed for CV")
	}
	// Require a version digit right after "%PDF-", e.g. "%PDF-1.7".
	if cvBytes[5] < '0' || cvBytes[5] > '9' {
		return errors.New("only PDF files are allowed for CV")
	}
	contentType := http.DetectContentType(cvBytes)
	if contentType != "application/pdf" {
		return errors.New("only PDF files are allowed for CV")
	}
	// A well-formed PDF ends with a %%EOF marker in its last 1024 bytes.
	// Rejecting files without it stops truncated uploads and plain-text
	// files that merely start with a "%PDF-" line.
	tail := cvBytes
	if len(tail) > 1024 {
		tail = tail[len(tail)-1024:]
	}
	if !bytes.Contains(tail, []byte("%%EOF")) {
		return errors.New("PDF file is incomplete (missing %%EOF marker)")
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
	// Reject PDFs carrying active content: embedded JavaScript actions or
	// launch actions that could execute when the file is opened.
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
	if len(trimmed) > maxDeveloperCVEncodedSize {
		return nil, errors.New("PDF max size is 5MB")
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

func currentUser(r *http.Request, svr server.Server) *middleware.UserJWT {
	profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
	if err != nil || profile == nil {
		return nil
	}
	return profile
}

// UploadDeveloperCVHandler handles POST /x/udp/cv: upload or replace the CV
// PDF of a developer profile. Only the profile owner or an admin may do so.
// The CV is accepted either as a multipart "cv" file or as a base64
// (optionally data:application/pdf;base64,...) "cv" JSON field.
func UploadDeveloperCVHandler(svr server.Server, devRepo developerCVRepository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile := currentUser(r, svr)
			if profile == nil {
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
				r.Body = http.MaxBytesReader(w, r.Body, maxDeveloperCVJSONRequestSize)
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

// DeleteDeveloperCVHandler handles DELETE /x/udp/cv: remove the stored CV
// PDF of a developer profile. Only the profile owner or an admin may do so.
func DeleteDeveloperCVHandler(svr server.Server, devRepo developerCVRepository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile := currentUser(r, svr)
			if profile == nil {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
			req := &struct {
				ID string `json:"id"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				svr.JSON(w, http.StatusBadRequest, "invalid request payload")
				return
			}
			profileID := strings.TrimSpace(req.ID)
			if profileID == "" {
				svr.JSON(w, http.StatusBadRequest, "profile id is required")
				return
			}
			dev, err := devRepo.DeveloperProfileByID(profileID)
			if err != nil {
				svr.Log(err, "unable to find developer profile for CV removal")
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			if !strings.EqualFold(dev.Email, profile.Email) && !profile.IsAdmin {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			if err := devRepo.DeleteDeveloperCV(profileID); err != nil {
				svr.Log(err, "unable to delete developer CV")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

// DownloadDeveloperCVHandler handles GET /developer/{slug}/cv: view or
// download a developer's CV PDF. Only authenticated recruiters, admins and
// the profile owner may access it. Every served download is recorded in the
// developer_cv_download audit table; if the audit write fails the download
// is refused (fail closed) so no download goes untracked. By default the PDF
// is served inline for in-browser viewing; ?download=1 serves it as an
// attachment instead.
func DownloadDeveloperCVHandler(svr server.Server, devRepo developerCVRepository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			profileSlug := vars["slug"]
			profile := currentUser(r, svr)
			if profile == nil {
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
			if err := devRepo.TrackDeveloperCVDownload(developer.DeveloperCVDownload{
				ID:                 downloadID.String(),
				DeveloperProfileID: dev.ID,
				CV:                 dev.CV,
				UserID:             profile.UserID,
			}); err != nil {
				// Fail closed: never serve a CV download that was not
				// recorded in the audit table.
				svr.Log(err, "unable to track developer CV download")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			disposition := "inline"
			if r.URL.Query().Get("download") == "1" {
				disposition = "attachment"
			}
			fileName := sanitizePDFFileName(dev.Slug)
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'")
			w.Header().Set("Cache-Control", "private, no-store")
			w.Header().Set("Content-Length", strconv.Itoa(len(dev.CV)))
			w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, fileName))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(dev.CV)
		},
	)
}

// ListDeveloperCVDownloadsHandler handles GET /x/udp/cv/downloads?id=...:
// return the CV download history (count + most recent downloads) for a
// developer profile. Only the profile owner or an admin may see it. The
// response never includes CV bytes.
func ListDeveloperCVDownloadsHandler(svr server.Server, devRepo developerCVRepository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile := currentUser(r, svr)
			if profile == nil {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			profileID := strings.TrimSpace(r.URL.Query().Get("id"))
			if profileID == "" {
				svr.JSON(w, http.StatusBadRequest, "profile id is required")
				return
			}
			dev, err := devRepo.DeveloperProfileByID(profileID)
			if err != nil {
				svr.Log(err, "unable to find developer profile for CV download history")
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			if !strings.EqualFold(dev.Email, profile.Email) && !profile.IsAdmin {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			limit := 20
			if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
				if parsed, err := strconv.Atoi(raw); err == nil {
					limit = parsed
				}
			}
			downloads, err := devRepo.GetDeveloperCVDownloads(profileID, limit)
			if err != nil {
				svr.Log(err, "unable to list developer CV downloads")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			count, err := devRepo.GetDeveloperCVDownloadCount(profileID)
			if err != nil {
				svr.Log(err, "unable to count developer CV downloads")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			svr.JSON(w, http.StatusOK, map[string]interface{}{
				"developer_profile_id": profileID,
				"download_count":       count,
				"downloads":            downloads,
			})
		},
	)
}
