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
	"sync"
	"time"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

const (
	defaultLinkedinUGCAPIURL = "https://api.linkedin.com/v2/ugcPosts"
	defaultLinkedinMeAPIURL  = "https://api.linkedin.com/v2/me"
	lastLinkedinJobIDMetaKey = "last_linkedin_job_id"
	linkedinRestliHeader     = "X-Restli-Protocol-Version"
	linkedinRestliVersion    = "2.0.0"
)

var linkedinSchedulerMu sync.Mutex

func parseLinkedinURNFromCandidate(raw string) string {
	candidate := strings.TrimSpace(raw)
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
	if !strings.Contains(trimmed, "/") && !strings.Contains(trimmed, ":") {
		return "urn:li:organization:" + trimmed
	}
	return ""
}

func resolveLinkedinAuthorURN(httpClient *http.Client, meEndpoint string, cfg config.Config) (string, error) {
	for _, candidate := range []string{cfg.LinkedinAuthorURN, cfg.LinkedinPageURL, cfg.SiteLinkedin} {
		if urn := parseLinkedinURNFromCandidate(candidate); urn != "" {
			return urn, nil
		}
	}
	token := strings.TrimSpace(cfg.LinkedinAccessToken)
	if token == "" {
		return "", fmt.Errorf("linkedin author URN and access token are empty")
	}
	if meEndpoint == "" {
		meEndpoint = defaultLinkedinMeAPIURL
	}
	req, err := http.NewRequest(http.MethodGet, meEndpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(linkedinRestliHeader, linkedinRestliVersion)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("linkedin /v2/me returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	var profile struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(bodyBytes, &profile); err != nil || strings.TrimSpace(profile.ID) == "" {
		return "", fmt.Errorf("unable to parse linkedin profile id from /v2/me")
	}
	return "urn:li:person:" + strings.TrimSpace(profile.ID), nil
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

func buildLinkedinUGCPayload(authorURN string, j job.JobPost, cfg config.Config) map[string]interface{} {
	jobURL := fmt.Sprintf("%s%s/job/%s", cfg.URLProtocol, cfg.SiteHost, j.Slug)
	text := formatLinkedinPostText(j, cfg)
	return map[string]interface{}{
		"author":         authorURN,
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

func postJobToLinkedin(httpClient *http.Client, endpoint, authorURN string, cfg config.Config, j job.JobPost) error {
	token := strings.TrimSpace(cfg.LinkedinAccessToken)
	if token == "" {
		return fmt.Errorf("linkedin access token is not configured")
	}
	if strings.TrimSpace(authorURN) == "" {
		return fmt.Errorf("linkedin author URN is not configured")
	}
	if endpoint == "" {
		endpoint = cfg.LinkedinAPIURL
	}
	if endpoint == "" {
		endpoint = defaultLinkedinUGCAPIURL
	}
	payload := buildLinkedinUGCPayload(authorURN, j, cfg)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(linkedinRestliHeader, linkedinRestliVersion)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("linkedin ugc api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
}

func runLinkedinScheduler(svr server.Server, jobRepo *job.Repository, httpClient *http.Client, ugcEndpoint, meEndpoint string) {
	if !linkedinSchedulerMu.TryLock() {
		log.Println("linkedin scheduler is already running; skipping concurrent trigger")
		return
	}
	defer linkedinSchedulerMu.Unlock()

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

	cfg := svr.GetConfig()
	jobsToPost := cfg.LinkedinJobsToPost
	if jobsToPost <= 0 {
		jobsToPost = cfg.TwitterJobsToPost
	}
	if jobsToPost <= 0 {
		jobsToPost = 2
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

	authorURN, err := resolveLinkedinAuthorURN(httpClient, meEndpoint, cfg)
	if err != nil {
		svr.Log(err, "unable to resolve linkedin author URN")
		return
	}

	lastJobID := lastLinkedinJobID
	postedCount := 0
	for _, j := range jobPosts {
		if j == nil {
			continue
		}
		if err := postJobToLinkedin(httpClient, ugcEndpoint, authorURN, cfg, *j); err != nil {
			svr.Log(err, "unable to post on linkedin")
			continue
		}
		if j.ID > lastJobID {
			lastJobID = j.ID
		}
		postedCount++
	}

	if postedCount == 0 {
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

func TriggerLinkedinShareJobs(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				runLinkedinScheduler(
					svr,
					jobRepo,
					&http.Client{Timeout: 15 * time.Second},
					svr.GetConfig().LinkedinAPIURL,
					defaultLinkedinMeAPIURL,
				)
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func TriggerLinkedinScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return TriggerLinkedinShareJobs(svr, jobRepo)
}
