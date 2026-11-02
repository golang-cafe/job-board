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

	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

const defaultLinkedInUGCPostsEndpoint = "https://api.linkedin.com/v2/ugcPosts"

var linkedInUGCPostsEndpoint = defaultLinkedInUGCPostsEndpoint

type linkedInText struct {
	Text string `json:"text"`
}

type linkedInMedia struct {
	Status      string        `json:"status"`
	Description *linkedInText `json:"description,omitempty"`
	OriginalURL string        `json:"originalUrl"`
	Title       *linkedInText `json:"title,omitempty"`
}

type linkedInShareContent struct {
	ShareCommentary    linkedInText    `json:"shareCommentary"`
	ShareMediaCategory string          `json:"shareMediaCategory"`
	Media              []linkedInMedia `json:"media,omitempty"`
}

type linkedInSpecificContent struct {
	ShareContent linkedInShareContent `json:"com.linkedin.ugc.ShareContent"`
}

type linkedInVisibility struct {
	MemberNetworkVisibility string `json:"com.linkedin.ugc.MemberNetworkVisibility"`
}

type linkedInUGCPostRequest struct {
	Author          string                  `json:"author"`
	LifecycleState  string                  `json:"lifecycleState"`
	SpecificContent linkedInSpecificContent `json:"specificContent"`
	Visibility      linkedInVisibility      `json:"visibility"`
}

func resolveLinkedInAuthorURN(pageOrURN string) string {
	trimmed := strings.TrimSpace(pageOrURN)
	if strings.HasPrefix(trimmed, "urn:li:") {
		return trimmed
	}
	trimmed = strings.TrimRight(trimmed, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx != -1 {
		trimmed = trimmed[idx+1:]
	}
	return fmt.Sprintf("urn:li:organization:%s", trimmed)
}

func formatLinkedInJobMessage(j job.JobPost, siteJobCategory, urlProtocol, siteHost string) string {
	return fmt.Sprintf(
		"%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s%s/job/%s",
		j.JobTitle,
		j.Company,
		j.Location,
		j.SalaryRange,
		siteJobCategory,
		siteJobCategory,
		urlProtocol,
		siteHost,
		j.Slug,
	)
}

func postJobToLinkedIn(client *http.Client, endpoint, apiToken, authorURN string, j job.JobPost, siteJobCategory, urlProtocol, siteHost string) error {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if endpoint == "" {
		endpoint = linkedInUGCPostsEndpoint
	}
	jobURL := fmt.Sprintf("%s%s/job/%s", urlProtocol, siteHost, j.Slug)
	commentary := formatLinkedInJobMessage(j, siteJobCategory, urlProtocol, siteHost)
	title := fmt.Sprintf("%s with %s - %s", j.JobTitle, j.Company, j.Location)
	payload := linkedInUGCPostRequest{
		Author:         resolveLinkedInAuthorURN(authorURN),
		LifecycleState: "PUBLISHED",
		SpecificContent: linkedInSpecificContent{
			ShareContent: linkedInShareContent{
				ShareCommentary:    linkedInText{Text: commentary},
				ShareMediaCategory: "ARTICLE",
				Media: []linkedInMedia{
					{
						Status:      "READY",
						Description: &linkedInText{Text: j.SalaryRange},
						OriginalURL: jobURL,
						Title:       &linkedInText{Text: title},
					},
				},
			},
		},
		Visibility: linkedInVisibility{
			MemberNetworkVisibility: "PUBLIC",
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("linkedin share failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

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
					svr.Log(err, "unable to convert job str to id")
					return
				}
				jobPosts, err := jobRepo.GetLastNJobsFromID(svr.GetConfig().TwitterJobsToPost, lastLinkedInJobID)
				if err != nil {
					svr.Log(err, "unable to retrieve last n jobs from id for linkedin")
					return
				}
				log.Printf("found %d/%d jobs to post on linkedin\n", len(jobPosts), svr.GetConfig().TwitterJobsToPost)
				if len(jobPosts) == 0 {
					return
				}
				lastJobID := lastLinkedInJobID
				client := &http.Client{Timeout: 15 * time.Second}
				for _, j := range jobPosts {
					err := postJobToLinkedIn(
						client,
						linkedInUGCPostsEndpoint,
						svr.GetConfig().LinkedinAPIToken,
						svr.GetConfig().LinkedinPageURL,
						*j,
						svr.GetConfig().SiteJobCategory,
						svr.GetConfig().URLProtocol,
						svr.GetConfig().SiteHost,
					)
					if err != nil {
						svr.Log(err, "unable to post on linkedin")
						continue
					}
					lastJobID = j.ID
				}
				lastJobIDStr := strconv.Itoa(lastJobID)
				err = jobRepo.SetValue("last_linkedin_job_id", lastJobIDStr)
				if err != nil {
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
