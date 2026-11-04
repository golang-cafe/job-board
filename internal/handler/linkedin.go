package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

const (
	defaultLinkedinUGCAPIURL = "https://api.linkedin.com/v2/ugcPosts"
	lastLinkedinJobIDMetaKey = "last_linkedin_job_id"
)

func resolveLinkedinAuthorURN(cfg config.Config) string {
	candidate := strings.TrimSpace(cfg.LinkedinAuthorURN)
	if candidate == "" {
		candidate = strings.TrimSpace(cfg.LinkedinPageURL)
	}
	if candidate == "" {
		candidate = strings.TrimSpace(cfg.SiteLinkedin)
	}
	if candidate == "" {
		return ""
	}
	if strings.HasPrefix(candidate, "urn:li:") {
		return candidate
	}
	trimmed := strings.TrimRight(candidate, "/")
	lower := strings.ToLower(trimmed)
	for _, marker := range []string{"/company/", "/organization/", "/school/"} {
		if idx := strings.Index(lower, marker); idx != -1 {
			slugOrID := trimmed[idx+len(marker):]
			if slashIdx := strings.Index(slugOrID, "/"); slashIdx != -1 {
				slugOrID = slugOrID[:slashIdx]
			}
			if qIdx := strings.Index(slugOrID, "?"); qIdx != -1 {
				slugOrID = slugOrID[:qIdx]
			}
			if slugOrID != "" {
				return "urn:li:organization:" + slugOrID
			}
		}
	}
	for _, marker := range []string{"/in/", "/person/"} {
		if idx := strings.Index(lower, marker); idx != -1 {
			slugOrID := trimmed[idx+len(marker):]
			if slashIdx := strings.Index(slugOrID, "/"); slashIdx != -1 {
				slugOrID = slugOrID[:slashIdx]
			}
			if qIdx := strings.Index(slugOrID, "?"); qIdx != -1 {
				slugOrID = slugOrID[:qIdx]
			}
			if slugOrID != "" {
				return "urn:li:person:" + slugOrID
			}
		}
	}
	return "urn:li:organization:" + trimmed
}

func formatLinkedinPostText(j job.JobPost, cfg config.Config) string {
	return fmt.Sprintf(
		"%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s%s/job/%s",
		j.JobTitle,
		j.Company,
		j.Location,
		j.SalaryRange,
		cfg.SiteJobCategory,
		cfg.SiteJobCategory,
		cfg.URLProtocol,
		cfg.SiteHost,
		j.Slug,
	)
}

func buildLinkedinUGCPayload(j job.JobPost, cfg config.Config) map[string]interface{} {
	jobURL := fmt.Sprintf("%s%s/job/%s", cfg.URLProtocol, cfg.SiteHost, j.Slug)
	text := formatLinkedinPostText(j, cfg)
	return map[string]interface{}{
		"author":         resolveLinkedinAuthorURN(cfg),
		"lifecycleState": "PUBLISHED",
		"specificContent": map[string]interface{}{
			"com.linkedin.ugc.ShareContent": map[string]interface{}{
				"shareCommentary": map[string]string{
					"text": text,
				},
				"shareMediaCategory": "ARTICLE",
				"media": []map[string]interface{}{
					{
						"status":      "READY",
						"originalUrl": jobURL,
						"title": map[string]string{
							"text": fmt.Sprintf("%s at %s", j.JobTitle, j.Company),
						},
						"description": map[string]string{
							"text": fmt.Sprintf("%s - %s | %s", j.Company, j.Location, j.SalaryRange),
						},
					},
				},
			},
		},
		"visibility": map[string]string{
			"com.linkedin.ugc.MemberNetworkVisibility": "PUBLIC",
		},
	}
}

func postJobToLinkedin(httpClient *http.Client, endpoint string, cfg config.Config, j job.JobPost) error {
	if endpoint == "" {
		endpoint = cfg.LinkedinAPIURL
	}
	if endpoint == "" {
		endpoint = defaultLinkedinUGCAPIURL
	}
	payload := buildLinkedinUGCPayload(j, cfg)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.LinkedinAccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("linkedin ugc api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}
	return nil
}

func runLinkedinScheduler(svr server.Server, jobRepo *job.Repository, httpClient *http.Client, endpoint string) {
	lastLinkedinJobIDStr, err := jobRepo.GetValue(lastLinkedinJobIDMetaKey)
	if err != nil {
		svr.Log(err, "unable to retrieve last linkedin job id")
		return
	}
	if strings.TrimSpace(lastLinkedinJobIDStr) == "" {
		lastLinkedinJobIDStr = "0"
	}
	lastLinkedinJobID, err := strconv.Atoi(lastLinkedinJobIDStr)
	if err != nil {
		svr.Log(err, "unable to convert linkedin job str to id")
		return
	}

	jobsToPost := svr.GetConfig().LinkedinJobsToPost
	if jobsToPost <= 0 {
		jobsToPost = svr.GetConfig().TwitterJobsToPost
	}
	if jobsToPost <= 0 {
		jobsToPost = 1
	}

	jobPosts, err := jobRepo.GetLastNJobsFromID(jobsToPost, lastLinkedinJobID)
	if err != nil {
		svr.Log(err, "unable to retrieve last n jobs from id for linkedin scheduler")
		return
	}
	log.Printf("found %d/%d jobs to post on linkedin\n", len(jobPosts), jobsToPost)
	if len(jobPosts) == 0 {
		return
	}

	lastJobID := lastLinkedinJobID
	postedCount := 0
	for _, j := range jobPosts {
		if err := postJobToLinkedin(httpClient, endpoint, svr.GetConfig(), *j); err != nil {
			svr.Log(err, "unable to post on linkedin")
			continue
		}
		lastJobID = j.ID
		postedCount++
	}

	if lastJobID == lastLinkedinJobID {
		return
	}

	lastJobIDStr := strconv.Itoa(lastJobID)
	if err := jobRepo.SetValue(lastLinkedinJobIDMetaKey, lastJobIDStr); err != nil {
		svr.Log(err, fmt.Sprintf("unable to save last linkedin job id to db as %s", lastJobIDStr))
		return
	}
	log.Printf("updated last linkedin job id to %s\n", lastJobIDStr)
	log.Printf("posted last %d jobs to linkedin\n", postedCount)
}

func TriggerLinkedinScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				runLinkedinScheduler(svr, jobRepo, &http.Client{Timeout: 15 * time.Second}, svr.GetConfig().LinkedinAPIURL)
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func TriggerLinkedinSchedulerHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return TriggerLinkedinScheduler(svr, jobRepo)
}
