package handler

import (
	"bytes"
	"encoding/json"
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

var linkedinUGCPostsEndpoint = "https://api.linkedin.com/v2/ugcPosts"

type linkedinShareMediaTitle struct {
	Text string `json:"text"`
}

type linkedinShareMedia struct {
	Status      string                  `json:"status"`
	OriginalURL string                  `json:"originalUrl"`
	Title       linkedinShareMediaTitle `json:"title"`
}

type linkedinShareCommentary struct {
	Text string `json:"text"`
}

type linkedinShareContent struct {
	ShareCommentary    linkedinShareCommentary `json:"shareCommentary"`
	ShareMediaCategory string                  `json:"shareMediaCategory"`
	Media              []linkedinShareMedia    `json:"media,omitempty"`
}

type linkedinUGCPostRequest struct {
	Author          string `json:"author"`
	LifecycleState  string `json:"lifecycleState"`
	SpecificContent struct {
		ShareContent linkedinShareContent `json:"com.linkedin.ugc.ShareContent"`
	} `json:"specificContent"`
	Visibility struct {
		MemberNetworkVisibility string `json:"com.linkedin.ugc.MemberNetworkVisibility"`
	} `json:"visibility"`
}

func resolveLinkedinAuthorURN(authorURN, pageURL string) string {
	trimmedURN := strings.TrimSpace(authorURN)
	if strings.HasPrefix(trimmedURN, "urn:li:") {
		return trimmedURN
	}
	if trimmedURN != "" {
		return fmt.Sprintf("urn:li:organization:%s", trimmedURN)
	}
	trimmedPage := strings.TrimRight(strings.TrimSpace(pageURL), "/")
	if strings.HasPrefix(trimmedPage, "urn:li:") {
		return trimmedPage
	}
	if idx := strings.LastIndex(trimmedPage, "/"); idx != -1 && idx+1 < len(trimmedPage) {
		return fmt.Sprintf("urn:li:organization:%s", trimmedPage[idx+1:])
	}
	if trimmedPage != "" {
		return fmt.Sprintf("urn:li:organization:%s", trimmedPage)
	}
	return ""
}

func formatLinkedinJobShareText(cfg config.Config, j *job.JobPost) (string, string) {
	jobURL := fmt.Sprintf("%s%s/job/%s", cfg.URLProtocol, cfg.SiteHost, j.Slug)
	text := fmt.Sprintf(
		"%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s",
		j.JobTitle,
		j.Company,
		j.Location,
		j.SalaryRange,
		cfg.SiteJobCategory,
		cfg.SiteJobCategory,
		jobURL,
	)
	return text, jobURL
}

func PostJobToLinkedin(client *http.Client, cfg config.Config, j *job.JobPost) error {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if strings.TrimSpace(cfg.LinkedinAccessToken) == "" {
		return fmt.Errorf("linkedin access token is empty")
	}
	author := resolveLinkedinAuthorURN(cfg.LinkedinAuthorURN, cfg.LinkedinPageURL)
	if author == "" {
		return fmt.Errorf("linkedin author URN or page URL is empty")
	}

	commentaryText, jobURL := formatLinkedinJobShareText(cfg, j)
	payload := linkedinUGCPostRequest{
		Author:         author,
		LifecycleState: "PUBLISHED",
	}
	payload.SpecificContent.ShareContent = linkedinShareContent{
		ShareCommentary: linkedinShareCommentary{
			Text: commentaryText,
		},
		ShareMediaCategory: "ARTICLE",
		Media: []linkedinShareMedia{
			{
				Status:      "READY",
				OriginalURL: jobURL,
				Title: linkedinShareMediaTitle{
					Text: fmt.Sprintf("%s with %s - %s", j.JobTitle, j.Company, j.Location),
				},
			},
		},
	}
	payload.Visibility.MemberNetworkVisibility = "PUBLIC"

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("unable to marshal linkedin share payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, linkedinUGCPostsEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("unable to create linkedin request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.LinkedinAccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("unable to execute linkedin share request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("linkedin share API returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func TriggerLinkedinScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				lastLinkedinJobIDStr, err := jobRepo.GetValue("last_linkedin_job_id")
				if err != nil {
					svr.Log(err, "unable to retrieve last linkedin job id")
					return
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
				jobPosts, err := jobRepo.GetLastNJobsFromID(jobsToPost, lastLinkedinJobID)
				if err != nil {
					svr.Log(err, "unable to retrieve jobs for linkedin scheduler")
					return
				}
				log.Printf("found %d/%d jobs to post on linkedin\n", len(jobPosts), jobsToPost)
				if len(jobPosts) == 0 {
					return
				}
				lastJobID := lastLinkedinJobID
				httpClient := &http.Client{Timeout: 15 * time.Second}
				for _, j := range jobPosts {
					if err := PostJobToLinkedin(httpClient, svr.GetConfig(), j); err != nil {
						svr.Log(err, "unable to post on linkedin")
						continue
					}
					lastJobID = j.ID
				}
				lastJobIDStr := strconv.Itoa(lastJobID)
				if err := jobRepo.SetValue("last_linkedin_job_id", lastJobIDStr); err != nil {
					svr.Log(err, fmt.Sprintf("unable to save last linkedin job id to db as %s", lastJobIDStr))
					return
				}
				log.Printf("updated last linkedin job id to %s\n", lastJobIDStr)
				log.Printf("posted last %d jobs to linkedin", len(jobPosts))
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}
