package handler

import (
	"errors"
	"net/http"

	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/golang-cafe/job-board/internal/twitter"
)

func TriggerTwitterJobsScheduler(svr server.Server) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(svr.GetConfig().MachineToken, func(w http.ResponseWriter, r *http.Request) {
		cfg := svr.GetConfig()
		if cfg.MachineToken == "" {
			svr.JSON(w, http.StatusServiceUnavailable, "scheduler is not configured")
			return
		}
		client, err := twitter.NewClient(cfg.TwitterClientKey, cfg.TwitterClientSecret, cfg.TwitterAccessToken, cfg.TwitterAccessTokenSecret)
		if err != nil {
			svr.JSON(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		posted, err := (twitter.Scheduler{DB: svr.Conn, Poster: client, SiteURL: cfg.URLProtocol + cfg.SiteHost}).Run(r.Context())
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, twitter.ErrPending) {
				status = http.StatusConflict
			}
			svr.JSON(w, status, map[string]interface{}{"posted": posted, "error": "scheduler stopped; inspect pending delivery before retrying"})
			return
		}
		svr.JSON(w, http.StatusOK, map[string]interface{}{"posted": posted})
	})
}
