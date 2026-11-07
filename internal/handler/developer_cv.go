package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/gorilla/mux"
	"github.com/segmentio/ksuid"
)

const (
	maxDeveloperCVSizeBytes = 5 * 1024 * 1024
	pdfMagicHeader          = "%PDF-"
)

// CVUploadRequest represents the JSON payload sent when uploading or updating a developer CV.
type CVUploadRequest struct {
	ProfileID string `json:"profile_id"`
	CVBase64  string `json:"cv_base64"`
}

// ValidateDeveloperCVBytes verifies that the uploaded CV is a non-empty PDF within the 5MB size limit.
func ValidateDeveloperCVBytes(cvBytes []byte) error {
	if len(cvBytes) == 0 {
		return fmt.Errorf("cv file cannot be empty")
	}
	if len(cvBytes) > maxDeveloperCVSizeBytes {
		return fmt.Errorf("cv file exceeds maximum size of 5MB")
	}
	if !bytes.HasPrefix(cvBytes, []byte(pdfMagicHeader)) {
		return fmt.Errorf("cv file must be a valid PDF document")
	}
	return nil
}

// DecodeBase64CV strips optional data-URI prefixes and decodes base64 CV bytes.
func DecodeBase64CV(rawBase64 string) ([]byte, error) {
	cleaned := strings.TrimSpace(rawBase64)
	if idx := strings.Index(cleaned, ","); idx != -1 {
		cleaned = cleaned[idx+1:]
	}
	return base64.StdEncoding.DecodeString(cleaned)
}

// FormatDeveloperCVStatus returns the human-readable CV availability label for profile views.
func FormatDeveloperCVStatus(cvBytes []byte) string {
	if len(cvBytes) == 0 {
		return "CV not uploaded"
	}
	return "CV uploaded"
}

func canUploadDeveloperCV(profile *middleware.UserJWT, err error) bool {
	if err != nil || profile == nil {
		return false
	}
	if profile.IsAdmin {
		return true
	}
	return profile.IsDeveloper
}

func canDownloadDeveloperCV(profile *middleware.UserJWT, err error) bool {
	if err != nil || profile == nil {
		return false
	}
	if profile.IsAdmin {
		return true
	}
	return profile.IsRecruiter
}

func parseAndValidateCVRequest(r *http.Request) ([]byte, error) {
	var req CVUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid request payload")
	}
	cvBytes, err := DecodeBase64CV(req.CVBase64)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 CV encoding")
	}
	if err := ValidateDeveloperCVBytes(cvBytes); err != nil {
		return nil, err
	}
	return cvBytes, nil
}

// UploadDeveloperCVHandler handles POST /x/udp/cv to upload or update a developer profile CV.
func UploadDeveloperCVHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if !canUploadDeveloperCV(profile, err) {
				svr.JSON(w, http.StatusForbidden, "only developers or admins can upload a CV")
				return
			}
			cvBytes, err := parseAndValidateCVRequest(r)
			if err != nil {
				svr.JSON(w, http.StatusBadRequest, err.Error())
				return
			}
			dev, err := devRepo.DeveloperProfileByEmail(profile.Email)
			if err != nil || dev.ID == "" {
				svr.JSON(w, http.StatusNotFound, "developer profile not found")
				return
			}
			if err := devRepo.SaveDeveloperCV(dev.ID, cvBytes); err != nil {
				svr.Log(err, "unable to save developer CV")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			svr.JSON(w, http.StatusOK, map[string]string{"status": FormatDeveloperCVStatus(cvBytes)})
		},
	)
}

// DownloadDeveloperCVHandler handles GET /developer/{id}/cv for logged-in recruiters and admins and records the download event.
func DownloadDeveloperCVHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if !canDownloadDeveloperCV(profile, err) {
				svr.JSON(w, http.StatusForbidden, "only logged-in recruiters or admins can download developer CVs")
				return
			}
			profileID := mux.Vars(r)["id"]
			cvBytes, err := devRepo.GetDeveloperCV(profileID)
			if err != nil || len(cvBytes) == 0 {
				svr.JSON(w, http.StatusNotFound, "CV not uploaded")
				return
			}
			_ = devRepo.RecordDeveloperCVDownload(developer.CVDownloadEvent{
				ID:                 ksuid.New().String(),
				DeveloperProfileID: profileID,
				CV:                 cvBytes,
				UserID:             profile.UserID,
				DownloadedAt:       time.Now().UTC(),
			})
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="developer-%s-cv.pdf"`, profileID))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(cvBytes)))
			_, _ = w.Write(cvBytes)
		},
	)
}
