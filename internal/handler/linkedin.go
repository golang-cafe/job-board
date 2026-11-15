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

const linkedInUGCPostsEndpoint = "https://api.linkedin.com/v2/ugcPosts"

func TriggerLinkedInShareJobs(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
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
				jobPosts, err := jobRepo.GetLastNJobsFromID(svr.GetConfig().TwitterJobsToPost, lastLinkedInJobID)
				if err != nil {
					svr.Log(err, "unable to retrieve last n jobs for linkedin")
					return
				}
				log.Printf("found %d/%d jobs to post on linkedin\n", len(jobPosts), svr.GetConfig().TwitterJobsToPost)
				if len(jobPosts) == 0 {
					return
				}
				lastJobID := lastLinkedInJobID
				for _, j := range jobPosts {
					jobURL := fmt.Sprintf("%s%s/job/%s", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, j.Slug)
					postText := fmt.Sprintf(
						"%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s",
						j.JobTitle,
						j.Company,
						j.Location,
						j.SalaryRange,
						svr.GetConfig().SiteJobCategory,
						svr.GetConfig().SiteJobCategory,
						jobURL,
					)
					if err := shareJobOnLinkedIn(svr.GetConfig(), postText, jobURL, j.JobTitle); err != nil {
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

func shareJobOnLinkedIn(cfg config.Config, text, jobURL, title string) error {
	authorURN := cfg.LinkedInAuthorURN
	if authorURN == "" && cfg.SiteLinkedin != "" {
		if strings.HasPrefix(cfg.SiteLinkedin, "urn:li:") {
			authorURN = cfg.SiteLinkedin
		} else {
			authorURN = fmt.Sprintf("urn:li:organization:%s", cfg.SiteLinkedin)
		}
	}

	payload := map[string]interface{}{
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
							"text": title,
						},
					},
				},
			},
		},
		"visibility": map[string]string{
			"com.linkedin.ugc.MemberNetworkVisibility": "PUBLIC",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, linkedInUGCPostsEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.LinkedInAPIToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("linkedin ugcPosts error (status %d): %s", resp.StatusCode, string(respBytes))
	}
	return nil
}
