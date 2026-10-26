package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"time"

	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/recruiter"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/gorilla/mux"
	"github.com/segmentio/ksuid"
)

const MaxDeveloperCVSize int64 = 5 * 1024 * 1024

var optionalLinkedinRegexp = regexp.MustCompile(`^https:\/\/(?:[a-z]{2,3}\.)?linkedin\.com\/.*$`)

// ValidateOptionalLinkedinURL allows an empty LinkedIn URL (optional on signup/edit)
// and validates the usual linkedin.com pattern when set.
func ValidateOptionalLinkedinURL(rawURL string) bool {
	if rawURL == "" {
		return true
	}
	return optionalLinkedinRegexp.MatchString(rawURL)
}

// ValidateDeveloperCVBytes enforces non-empty, max-size, application/pdf CV uploads.
func ValidateDeveloperCVBytes(fileBytes []byte, maxBytes int64) (string, int) {
	if len(fileBytes) == 0 {
		return "empty cv file", http.StatusBadRequest
	}
	if int64(len(fileBytes)) > maxBytes {
		return "cv file exceeds maximum size", http.StatusRequestEntityTooLarge
	}
	contentType := http.DetectContentType(fileBytes)
	if contentType != "application/pdf" {
		return fmt.Sprintf("PDF file is not application/pdf got %s", contentType), http.StatusUnsupportedMediaType
	}
	return "", http.StatusOK
}

// UploadDeveloperProfileCvHandler stores a PDF CV (bytea) on the developer's profile.
func UploadDeveloperProfileCvHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil || (!profile.IsAdmin && !profile.IsDeveloper) {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, MaxDeveloperCVSize)
			cvFile, header, err := r.FormFile("cv")
			if err != nil {
				svr.Log(err, "unable to read developer cv file")
				svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
				return
			}
			defer cvFile.Close()

			fileBytes, err := ioutil.ReadAll(cvFile)
			if err != nil {
				svr.Log(err, "unable to read developer cv file content")
				svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
				return
			}
			if header.Size > MaxDeveloperCVSize {
				svr.Log(errors.New("developer CV file is too large"), fmt.Sprintf("PDF file too large: %d > %d", header.Size, MaxDeveloperCVSize))
				svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
				return
			}
			if errMsg, status := ValidateDeveloperCVBytes(fileBytes, MaxDeveloperCVSize); status != http.StatusOK {
				svr.Log(errors.New(errMsg), errMsg)
				svr.JSON(w, status, nil)
				return
			}

			profileID := r.FormValue("profile-id")
			var dev developer.Developer
			if profileID != "" {
				dev, err = devRepo.DeveloperProfileByID(profileID)
			} else {
				dev, err = devRepo.DeveloperProfileByEmail(profile.Email)
			}
			if err != nil || dev.ID == "" {
				svr.Log(err, "unable to find developer profile for CV upload")
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			if dev.Email != profile.Email && !profile.IsAdmin {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}

			if err := devRepo.UpdateDeveloperCV(dev.ID, fileBytes); err != nil {
				svr.Log(err, "unable to update developer profile CV")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}

			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

// ViewDeveloperProfileCvHandler serves a developer CV to recruiters/admins (active plan)
// or the profile owner, and audits the download with a CV snapshot.
func ViewDeveloperProfileCvHandler(svr server.Server, devRepo *developer.Repository, recRepo *recruiter.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil || profile == nil {
				svr.Redirect(w, r, http.StatusTemporaryRedirect, "/auth")
				return
			}

			vars := mux.Vars(r)
			profileSlug := vars["slug"]
			dev, cvBytes, cvSize, err := devRepo.GetDeveloperCVBySlug(profileSlug)
			if err != nil || cvSize == 0 {
				svr.Log(err, fmt.Sprintf("unable to find developer CV by slug: %s", profileSlug))
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}

			isOwner := profile.IsDeveloper && dev.Email == profile.Email
			if !profile.IsAdmin && !profile.IsRecruiter && !isOwner {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}

			if profile.IsRecruiter && !profile.IsAdmin {
				expTime, err := recRepo.RecruiterProfilePlanExpiration(profile.Email)
				if err != nil || expTime.Before(time.Now().UTC()) {
					svr.Redirect(w, r, http.StatusTemporaryRedirect, fmt.Sprintf("%s%s/profile/home#developer-subscription", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost))
					return
				}
			}

			k, kErr := ksuid.NewRandom()
			if kErr == nil {
				if trackErr := devRepo.TrackDeveloperCVDownload(k.String(), dev.ID, cvBytes, profile.UserID); trackErr != nil {
					svr.Log(trackErr, fmt.Sprintf("unable to track developer CV download for profile %s", dev.ID))
				}
			}

			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", cvSize))
			if _, err := w.Write(cvBytes); err != nil {
				svr.Log(err, fmt.Sprintf("unable to serve developer CV for slug: %s", profileSlug))
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		},
	)
}

func ReceivedMessages(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, _ := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if !profile.IsAdmin && !profile.IsDeveloper {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}

			dev, err := devRepo.DeveloperProfileByEmail(profile.Email)
			if err != nil {
				svr.Log(err, "DeveloperProfileByEmail")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}

			messages, err := devRepo.GetDeveloperMessagesSentTo(dev.ID)
			if err != nil {
				svr.Log(err, "GetDeveloperMessagesSentTo")
			}

			viewCount, err := devRepo.GetViewCountForProfile(dev.ID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve job view count for dev id %s", dev.ID))
			}
			messagesCount, err := devRepo.GetMessagesCountForJob(dev.ID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve job messages count for dev id %s", dev.ID))
			}
			stats, err := devRepo.GetStatsForProfile(dev.ID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve stats for dev id %s", dev.ID))
			}
			statsSet, err := json.Marshal(stats)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to marshal stats for dev id %s", dev.ID))
			}

			err = svr.Render(r, w, http.StatusOK, "messages.html", map[string]interface{}{
				"Messages":      messages,
				"ViewCount":     viewCount,
				"MessagesCount": messagesCount,
				"Stats":         string(statsSet),
			})
			if err != nil {
				svr.Log(err, "unable to render sent messages page")
			}
		})
}
