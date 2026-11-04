package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/golang-cafe/job-board/internal/linkedin"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

func TriggerLinkedinShareJobs(svr server.Server) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(svr.GetConfig().MachineToken, func(w http.ResponseWriter, r *http.Request) {
		cfg := svr.GetConfig()
		if cfg.MachineToken == "" {
			svr.JSON(w, http.StatusServiceUnavailable, "task is not configured")
			return
		}
		client, err := linkedin.NewClient(cfg.LinkedinAccessToken, cfg.LinkedinAuthorURN, cfg.LinkedinAPIVersion)
		if err != nil {
			svr.JSON(w, http.StatusServiceUnavailable, "LinkedIn is not configured")
			return
		}
		scheduler := linkedin.Scheduler{DB: svr.Conn, Poster: client, SiteURL: cfg.URLProtocol + cfg.SiteHost}
		delivery, err := scheduler.Reserve(r.Context())
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, linkedin.ErrPending) {
				status = http.StatusConflict
			}
			svr.JSON(w, status, "task unavailable; inspect LinkedIn pending state")
			return
		}
		if delivery == nil {
			svr.JSON(w, http.StatusOK, map[string]interface{}{"queued": 0})
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := scheduler.Send(ctx, delivery); err != nil {
				// SQL/provider errors may contain credentials or content. Keep logs fixed.
				svr.Log(errors.New("LinkedIn delivery remains pending"), "inspect pending job before another trigger")
			}
		}()
		svr.JSON(w, http.StatusAccepted, map[string]interface{}{"queued": 1, "job_id": delivery.ID})
	})
}
