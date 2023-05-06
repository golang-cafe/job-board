package handler

import (
	"encoding/json"
	"fmt"
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
