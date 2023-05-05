package handler

import (
	"net/http"

	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

func SentMessages(svr server.Server) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, _ := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if !profile.IsAdmin && !profile.IsRecruiter {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			svr.Render(r, w, http.StatusOK, "sent-messages.html", map[string]interface{}{})
		})
}
