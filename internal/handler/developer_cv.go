package handler

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"net/http"

	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/gorilla/mux"
)

const maxCVBytes = 5 * 1024 * 1024

type developerCVStore interface {
	SaveCV(context.Context, string, []byte) error
	DownloadCV(context.Context, string, string) ([]byte, error)
}

func validCV(data []byte) bool {
	return len(data) > 0 && len(data) <= maxCVBytes && bytes.HasPrefix(data, []byte("%PDF-")) && http.DetectContentType(data) == "application/pdf"
}

func UploadDeveloperCV(svr server.Server, store developerCVStore) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(svr.SessionStore, svr.GetJWTSigningKey(), func(w http.ResponseWriter, r *http.Request) {
		profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
		if err != nil || !profile.IsDeveloper {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		// A custom header prevents cross-origin form submissions with session cookies.
		if r.Header.Get("X-CV-Upload") != "1" {
			http.Error(w, "missing upload header", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxCVBytes+1024*1024)
		if err := r.ParseMultipartForm(maxCVBytes); err != nil {
			http.Error(w, "invalid or oversized upload", http.StatusBadRequest)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, _, err := r.FormFile("cv")
		if err != nil {
			http.Error(w, "CV file required", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxCVBytes+1))
		if err != nil || !validCV(data) {
			http.Error(w, "CV must be a PDF up to 5 MiB", http.StatusBadRequest)
			return
		}
		if err = store.SaveCV(r.Context(), profile.Email, data); err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
			} else {
				http.Error(w, "unable to save CV", http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func DownloadDeveloperCV(svr server.Server, store developerCVStore) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(svr.SessionStore, svr.GetJWTSigningKey(), func(w http.ResponseWriter, r *http.Request) {
		profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
		if err != nil || profile.UserID == "" || (!profile.IsAdmin && !profile.IsRecruiter) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		data, err := store.DownloadCV(r.Context(), mux.Vars(r)["id"], profile.UserID)
		if err != nil {
			if err == sql.ErrNoRows {
				http.NotFound(w, r)
			} else {
				http.Error(w, "unable to download CV", http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="cv.pdf"`)
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Write(data)
	})
}
