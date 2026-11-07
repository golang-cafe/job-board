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

const linkedInUGCPostsEndpoint = "https://api.linkedin.com/v2/ugcPosts"

// linkedInClient encapsulates OAuth2 Bearer credentials and author URN configuration for LinkedIn ugcPosts shares.
type linkedInClient struct {
	accessToken string
	authorURN   string
	endpoint    string
	httpClient  *http.Client
}

// linkedInShareJobRequest carries the commentary and canonical job URL for a LinkedIn ARTICLE share.
type linkedInShareJobRequest struct {
	commentary string
	jobURL     string
	title      string
}

// linkedInBatchRunner coordinates job retrieval, LinkedIn ugcPosts publication, and cursor updates.
type linkedInBatchRunner struct {
	svr     server.Server
	jobRepo *job.Repository
	client  *linkedInClient
}

// newLinkedInClient initializes a LinkedIn API client from site configuration.
func newLinkedInClient(cfg config.Config) *linkedInClient {
	pageRef := cfg.LinkedinPageURL
	if pageRef == "" {
		pageRef = cfg.SiteLinkedin
	}
	return &linkedInClient{
		accessToken: cfg.LinkedinAccessToken,
		authorURN:   normalizeLinkedInAuthorURN(pageRef),
		endpoint:    linkedInUGCPostsEndpoint,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

// normalizeLinkedInAuthorURN resolves a LinkedIn organization/person URN from a raw URN, page URL, or company identifier.
func normalizeLinkedInAuthorURN(raw string) string {
	trimmed := strings.TrimSpace(strings.TrimRight(raw, "/"))
	if strings.HasPrefix(trimmed, "urn:li:") {
		return trimmed
	}
	if idx := strings.LastIndex(trimmed, "/"); idx != -1 {
		trimmed = trimmed[idx+1:]
	}
	if trimmed == "" {
		return "urn:li:organization:0"
	}
	return "urn:li:organization:" + trimmed
}

// formatLinkedInJobPost builds the job announcement commentary and canonical permalink for LinkedIn sharing.
func formatLinkedInJobPost(cfg config.Config, j job.JobPost) linkedInShareJobRequest {
	jobURL := fmt.Sprintf("%s%s/job/%s", cfg.URLProtocol, cfg.SiteHost, j.Slug)
	commentary := fmt.Sprintf(
		"%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s",
		j.JobTitle,
		j.Company,
		j.Location,
		j.SalaryRange,
		cfg.SiteJobCategory,
		cfg.SiteJobCategory,
		jobURL,
	)
	return linkedInShareJobRequest{
		commentary: commentary,
		jobURL:     jobURL,
		title:      fmt.Sprintf("%s at %s", j.JobTitle, j.Company),
	}
}

// TriggerLinkedInShareJobs handles /x/task/linkedin-share-jobs and publishes unshared jobs to LinkedIn.
func TriggerLinkedInShareJobs(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	runner := &linkedInBatchRunner{
		svr:     svr,
		jobRepo: jobRepo,
		client:  newLinkedInClient(svr.GetConfig()),
	}
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go runner.runBatch()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func (r *linkedInBatchRunner) runBatch() {
	lastLinkedinJobID, err := loadLastLinkedInJobID(r.jobRepo)
	if err != nil {
		r.svr.Log(err, "unable to retrieve last linkedin job id")
		return
	}
	limit := r.svr.GetConfig().TwitterJobsToPost
	jobPosts, err := r.jobRepo.GetLastNJobsFromID(limit, lastLinkedinJobID)
	if err != nil {
		r.svr.Log(err, "unable to query jobs for linkedin scheduler")
		return
	}
	log.Printf("found %d/%d jobs to post on linkedin\n", len(jobPosts), limit)
	if len(jobPosts) == 0 {
		return
	}
	updatedID := r.publishJobs(jobPosts, lastLinkedinJobID)
	r.saveLastJobID(updatedID, len(jobPosts))
}

func loadLastLinkedInJobID(jobRepo *job.Repository) (int, error) {
	rawID, err := jobRepo.GetValue("last_linkedin_job_id")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(rawID)
}

func (r *linkedInBatchRunner) publishJobs(jobs []job.JobPost, initialID int) int {
	lastID := initialID
	for _, j := range jobs {
		shareReq := formatLinkedInJobPost(r.svr.GetConfig(), j)
		if err := r.client.shareJobPost(shareReq); err != nil {
			r.svr.Log(err, "unable to post on linkedin")
			continue
		}
		lastID = j.ID
	}
	return lastID
}

func (r *linkedInBatchRunner) saveLastJobID(lastID int, count int) {
	lastJobIDStr := strconv.Itoa(lastID)
	if err := r.jobRepo.SetValue("last_linkedin_job_id", lastJobIDStr); err != nil {
		r.svr.Log(err, fmt.Sprintf("unable to save last linkedin job id to db as %s", lastJobIDStr))
		return
	}
	log.Printf("updated last linkedin job id to %s\n", lastJobIDStr)
	log.Printf("posted last %d jobs to linkedin", count)
}

// shareJobPost sends an ARTICLE ugcPost request to the LinkedIn Share on LinkedIn v2 API.
func (c *linkedInClient) shareJobPost(reqData linkedInShareJobRequest) error {
	payloadBytes, err := json.Marshal(c.buildUGCPayload(reqData))
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return validateLinkedInResponse(resp)
}

func (c *linkedInClient) buildUGCPayload(reqData linkedInShareJobRequest) map[string]interface{} {
	mediaEntry := map[string]interface{}{
		"status":      "READY",
		"originalUrl": reqData.jobURL,
		"title": map[string]string{
			"text": reqData.title,
		},
	}
	shareContent := map[string]interface{}{
		"shareCommentary": map[string]string{
			"text": reqData.commentary,
		},
		"shareMediaCategory": "ARTICLE",
		"media":              []interface{}{mediaEntry},
	}
	return map[string]interface{}{
		"author":         c.authorURN,
		"lifecycleState": "PUBLISHED",
		"specificContent": map[string]interface{}{
			"com.linkedin.ugc.ShareContent": shareContent,
		},
		"visibility": map[string]string{
			"com.linkedin.ugc.MemberNetworkVisibility": "PUBLIC",
		},
	}
}

func validateLinkedInResponse(resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("linkedin api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
}
