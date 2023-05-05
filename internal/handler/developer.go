package handler

import (
	"net/http"

	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

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

			err = svr.Render(r, w, http.StatusOK, "messages.html", map[string]interface{}{
				"Messages": messages,
			})
			if err != nil {
				svr.Log(err, "unable to render sent messages page")
			}
		})
}
