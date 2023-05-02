package bookmark

import (
	"net/http"

	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

func BookmarksHandler(svr server.Server, bookmarkRepo *Repository) http.HandlerFunc {
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
