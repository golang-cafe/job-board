package handler

import (
	"net/http"

	"github.com/golang-cafe/job-board/internal/bookmark"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

func BookmarkListHandler(svr server.Server, bookmarkRepo *bookmark.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
		if err != nil {
			svr.Log(err, "unable to retrieve user from JWT")
			svr.JSON(w, http.StatusForbidden, nil)
			return
		}

		bookmarks, err := bookmarkRepo.GetBookmarksForUser(profile.UserID)
		if err != nil {
			svr.Log(err, "GetBookmarksForUser")
		}

		err = svr.Render(r, w, http.StatusOK, "bookmarks.html", map[string]interface{}{
			"Bookmarks": bookmarks,
		})
		if err != nil {
			svr.Log(err, "unable to render bookmarks page")
		}
	}
}

func BookmarkJobHandler(svr server.Server, bookmarkRepo *bookmark.Repository, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
		if err != nil {
			svr.Log(err, "unable to retrieve user from JWT")
			svr.JSON(w, http.StatusForbidden, nil)
			return
		}

		externalID := r.FormValue("job-id")
		job, err := jobRepo.GetJobByExternalID(externalID)
		if err != nil {
			svr.Log(err, "BookmarkJob")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}

		switch r.Method {
		case http.MethodPost:
			err = bookmarkRepo.BookmarkJob(profile.UserID, job.ID, false)
			if err != nil {
				svr.Log(err, "BookmarkJob")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			svr.JSON(w, http.StatusCreated, nil)

		case http.MethodDelete:
			err = bookmarkRepo.RemoveBookmark(profile.UserID, job.ID)
			if err != nil {
				svr.Log(err, "RemoveBookmark")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			svr.JSON(w, http.StatusNoContent, nil)
		}
	}
}
