package handler

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/segmentio/ksuid"

	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

const (
	maxDeveloperCVSize  = 5 * 1024 * 1024
	pdfContentTypeValue = "application/pdf"
)

type developerCVStore interface {
	DeveloperProfileByID(id string) (developer.Developer, error)
	DeveloperProfileBySlug(slug string) (developer.Developer, error)
	SaveDeveloperCV(profileID string, cv []byte) error
	TrackDeveloperCVDownload(id, profileID string, cv []byte, userID string) error
}

func UploadDeveloperCVHandler(svr server.Server, devRepo developerCVStore) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			handleUploadDeveloperCV(svr, devRepo, w, r)
		},
	)
}

func handleUploadDeveloperCV(svr server.Server, devRepo developerCVStore, w http.ResponseWriter, r *http.Request) {
	profile, authErr := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
	if authErr != nil || profile == nil {
		svr.JSON(w, http.StatusForbidden, nil)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxDeveloperCVSize))
	cvFile, header, fileErr := r.FormFile("cv")
	if fileErr != nil {
		svr.Log(fileErr, "unable to read developer cv file")
		svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
		return
	}
	defer cvFile.Close()
	if header.Size > int64(maxDeveloperCVSize) {
		svr.Log(errors.New("developer CV file is too large"), "developer CV file exceeds maximum size")
		svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
		return
	}
	fileBytes, readErr := ioutil.ReadAll(cvFile)
	if readErr != nil {
		svr.Log(readErr, "unable to read developer cv file content")
		svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
		return
	}
	contentType := http.DetectContentType(fileBytes)
	if contentType != pdfContentTypeValue {
		svr.Log(errors.New("developer CV is not application/pdf"), "invalid developer CV content type")
		svr.JSON(w, http.StatusUnsupportedMediaType, nil)
		return
	}
	profileID := r.FormValue("profile_id")
	dev, lookupErr := devRepo.DeveloperProfileByID(profileID)
	if lookupErr != nil || dev.ID == "" {
		svr.Log(lookupErr, "unable to find developer profile for cv upload")
		svr.JSON(w, http.StatusNotFound, nil)
		return
	}
	if dev.Email != profile.Email && !profile.IsAdmin {
		svr.JSON(w, http.StatusForbidden, nil)
		return
	}
	if saveErr := devRepo.SaveDeveloperCV(dev.ID, fileBytes); saveErr != nil {
		svr.Log(saveErr, "unable to save developer cv")
		svr.JSON(w, http.StatusInternalServerError, nil)
		return
	}
	svr.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func DownloadDeveloperCVHandler(svr server.Server, devRepo developerCVStore) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			handleDownloadDeveloperCV(svr, devRepo, w, r)
		},
	)
}

func handleDownloadDeveloperCV(svr server.Server, devRepo developerCVStore, w http.ResponseWriter, r *http.Request) {
	profile, authErr := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
	if authErr != nil || profile == nil {
		svr.JSON(w, http.StatusForbidden, nil)
		return
	}
	if !profile.IsRecruiter && !profile.IsAdmin {
		svr.JSON(w, http.StatusForbidden, nil)
		return
	}
	vars := mux.Vars(r)
	profileSlug := vars["slug"]
	dev, lookupErr := devRepo.DeveloperProfileBySlug(profileSlug)
	if lookupErr != nil || dev.ID == "" {
		svr.Log(lookupErr, "unable to find developer profile for cv download")
		svr.JSON(w, http.StatusNotFound, nil)
		return
	}
	if len(dev.CV) == 0 {
		svr.JSON(w, http.StatusNotFound, "CV not uploaded")
		return
	}
	downloadID, idErr := ksuid.NewRandom()
	if idErr != nil {
		svr.Log(idErr, "unable to generate cv download id")
		svr.JSON(w, http.StatusInternalServerError, nil)
		return
	}
	if trackErr := devRepo.TrackDeveloperCVDownload(downloadID.String(), dev.ID, dev.CV, profile.UserID); trackErr != nil {
		svr.Log(trackErr, "unable to track developer cv download")
	}
	w.Header().Set("Content-Type", pdfContentTypeValue)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", dev.Slug+"-cv.pdf"))
	w.WriteHeader(http.StatusOK)
	if _, writeErr := w.Write(dev.CV); writeErr != nil {
		svr.Log(writeErr, "unable to write developer cv response")
	}
}
