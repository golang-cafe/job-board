package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
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

const defaultLinkedInUGCPostsEndpoint = "https://api.linkedin.com/v2/ugcPosts"

type linkedInUGCPostRequest struct {
	Author          string                     `json:"author"`
	LifecycleState  string                     `json:"lifecycleState"`
	SpecificContent linkedInUGCSpecificContent `json:"specificContent"`
	Visibility      linkedInUGCPostVisibility  `json:"visibility"`
}

type linkedInUGCSpecificContent struct {
	ShareContent linkedInUGCShareContent `json:"com.linkedin.ugc.ShareContent"`
}

type linkedInUGCShareContent struct {
	ShareCommentary    linkedInUGCText    `json:"shareCommentary"`
	ShareMediaCategory string             `json:"shareMediaCategory"`
	Media              []linkedInUGCMedia `json:"media,omitempty"`
}

type linkedInUGCMedia struct {
	Status      string           `json:"status"`
	Description *linkedInUGCText `json:"description,omitempty"`
	OriginalURL string           `json:"originalUrl"`
	Title       *linkedInUGCText `json:"title,omitempty"`
}

type linkedInUGCText struct {
	Text string `json:"text"`
}

type linkedInUGCPostVisibility struct {
	MemberNetworkVisibility string `json:"com.linkedin.ugc.MemberNetworkVisibility"`
}

func normalizeLinkedInAuthorURN(raw string) string {
	val := strings.TrimSpace(raw)
	if val == "" {
		return ""
	}
	if strings.HasPrefix(val, "urn:li:") {
		return val
	}
	trimmed := strings.TrimRight(val, "/")
	if idx := strings.Index(trimmed, "/in/"); idx != -1 {
		personID := strings.Trim(trimmed[idx+len("/in/"):], "/")
		if personID != "" {
			return "urn:li:person:" + personID
		}
	}
	for _, marker := range []string{"/company/", "/organization/"} {
		if idx := strings.Index(trimmed, marker); idx != -1 {
			orgID := strings.Trim(trimmed[idx+len(marker):], "/")
			if orgID != "" {
				return "urn:li:organization:" + orgID
			}
		}
	}
	if idx := strings.LastIndex(trimmed, "/"); idx != -1 {
		trimmed = trimmed[idx+1:]
	}
	if trimmed == "" {
		return ""
	}
	return "urn:li:organization:" + trimmed
}

func formatLinkedInJobCommentary(j *job.JobPost, cfg config.Config) string {
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

func postLinkedInUGCPost(client *http.Client, endpoint string, cfg config.Config, j *job.JobPost) error {
	token := strings.TrimSpace(cfg.LinkedInAPIToken)
	if token == "" {
		return errors.New("linkedin api token cannot be empty")
	}
	authorSource := cfg.LinkedInPageURL
	if strings.TrimSpace(authorSource) == "" {
		authorSource = cfg.SiteLinkedin
	}
	authorURN := normalizeLinkedInAuthorURN(authorSource)
	if authorURN == "" {
		return errors.New("linkedin page url or author URN cannot be empty")
	}
	if endpoint == "" {
		endpoint = defaultLinkedInUGCPostsEndpoint
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	jobURL := fmt.Sprintf("%s%s/job/%s", cfg.URLProtocol, cfg.SiteHost, j.Slug)
	commentary := formatLinkedInJobCommentary(j, cfg)
	titleText := fmt.Sprintf("%s with %s", j.JobTitle, j.Company)
	descText := fmt.Sprintf("%s | %s", j.Location, j.SalaryRange)

	payload := linkedInUGCPostRequest{
		Author:         authorURN,
		LifecycleState: "PUBLISHED",
		SpecificContent: linkedInUGCSpecificContent{
			ShareContent: linkedInUGCShareContent{
				ShareCommentary:    linkedInUGCText{Text: commentary},
				ShareMediaCategory: "ARTICLE",
				Media: []linkedInUGCMedia{
					{
						Status:      "READY",
						Description: &linkedInUGCText{Text: descText},
						OriginalURL: jobURL,
						Title:       &linkedInUGCText{Text: titleText},
					},
				},
			},
		},
		Visibility: linkedInUGCPostVisibility{
			MemberNetworkVisibility: "PUBLIC",
		},
	}

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
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("linkedin ugcPosts api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func runLinkedInScheduler(svr server.Server, jobRepo *job.Repository, client *http.Client, endpoint string) {
	lastLinkedInJobIDStr, err := jobRepo.GetValue("last_linkedin_job_id")
	if err != nil {
		svr.Log(err, "unable to retrieve last linkedin job id")
		return
	}
	lastLinkedInJobID, err := strconv.Atoi(lastLinkedInJobIDStr)
	if err != nil {
		svr.Log(err, "unable to convert linkedin job str to id")
		return
	}
	jobsToPost := svr.GetConfig().LinkedInJobsToPost
	if jobsToPost <= 0 {
		jobsToPost = svr.GetConfig().TwitterJobsToPost
	}
	jobPosts, err := jobRepo.GetLastNJobsFromID(jobsToPost, lastLinkedInJobID)
	if err != nil {
		svr.Log(err, "unable to retrieve jobs for linkedin scheduler")
		return
	}
	log.Printf("found %d/%d jobs to post on linkedin\n", len(jobPosts), jobsToPost)
	if len(jobPosts) == 0 {
		return
	}
	lastJobID := lastLinkedInJobID
	postedCount := 0
	for _, j := range jobPosts {
		if err := postLinkedInUGCPost(client, endpoint, svr.GetConfig(), j); err != nil {
			svr.Log(err, "unable to post on linkedin")
			continue
		}
		lastJobID = j.ID
		postedCount++
	}
	lastJobIDStr := strconv.Itoa(lastJobID)
	err = jobRepo.SetValue("last_linkedin_job_id", lastJobIDStr)
	if err != nil {
		svr.Log(err, fmt.Sprintf("unable to save last linkedin job id to db as %s", lastJobIDStr))
		return
	}
	log.Printf("updated last linkedin job id to %s\n", lastJobIDStr)
	log.Printf("posted last %d jobs to linkedin", postedCount)
}

func TriggerLinkedInShareJobs(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				runLinkedInScheduler(svr, jobRepo, &http.Client{Timeout: 15 * time.Second}, defaultLinkedInUGCPostsEndpoint)
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}
