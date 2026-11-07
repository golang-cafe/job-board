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

const (
	defaultLinkedinUGCEndpoint = "https://api.linkedin.com/v2/ugcPosts"
	defaultLinkedinMeEndpoint  = "https://api.linkedin.com/v2/me"
	defaultLinkedinPostsLimit  = 2
	lastLinkedinJobIDMetaKey   = "last_linkedin_job_id"
	linkedinRestliHeader       = "X-Restli-Protocol-Version"
	linkedinRestliVersion      = "2.0.0"
	linkedinURNPrefix          = "urn:li:"
	linkedinPersonURNPrefix    = "urn:li:person:"
	linkedinOrgURNPrefix       = "urn:li:organization:"
)

type linkedinJobStore interface {
	GetValue(key string) (string, error)
	SetValue(key, val string) error
	GetLastNJobsFromID(max, jobID int) ([]job.JobPost, error)
}

type linkedinClient struct {
	ugcEndpoint string
	meEndpoint  string
	accessToken string
	authorURN   string
	pageURL     string
	httpClient  *http.Client
}

type linkedinTextValue struct {
	Text string `json:"text"`
}

type linkedinShareMedia struct {
	Status      string             `json:"status"`
	Description *linkedinTextValue `json:"description,omitempty"`
	OriginalURL string             `json:"originalUrl"`
	Title       *linkedinTextValue `json:"title,omitempty"`
}

type linkedinShareContent struct {
	ShareCommentary    linkedinTextValue    `json:"shareCommentary"`
	ShareMediaCategory string               `json:"shareMediaCategory"`
	Media              []linkedinShareMedia `json:"media,omitempty"`
}

type linkedinUGCPostRequest struct {
	Author          string                          `json:"author"`
	LifecycleState  string                          `json:"lifecycleState"`
	SpecificContent map[string]linkedinShareContent `json:"specificContent"`
	Visibility      map[string]string               `json:"visibility"`
}

func newLinkedinClient(accessToken, authorURN, pageURL string) *linkedinClient {
	return &linkedinClient{
		ugcEndpoint: defaultLinkedinUGCEndpoint,
		meEndpoint:  defaultLinkedinMeEndpoint,
		accessToken: strings.TrimSpace(accessToken),
		authorURN:   strings.TrimSpace(authorURN),
		pageURL:     strings.TrimSpace(pageURL),
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

func deriveURNFromPageURL(pageURL string) string {
	trimmed := strings.TrimRight(pageURL, "/")
	if strings.HasPrefix(trimmed, linkedinURNPrefix) {
		return trimmed
	}
	parts := strings.Split(trimmed, "/")
	slugOrID := parts[len(parts)-1]
	if slugOrID == "" {
		return ""
	}
	if strings.Contains(trimmed, "/in/") {
		return linkedinPersonURNPrefix + slugOrID
	}
	return linkedinOrgURNPrefix + slugOrID
}

func (c *linkedinClient) resolveAuthorURN() (string, error) {
	if c.authorURN != "" {
		if strings.HasPrefix(c.authorURN, linkedinURNPrefix) {
			return c.authorURN, nil
		}
		return linkedinOrgURNPrefix + c.authorURN, nil
	}
	if c.pageURL != "" {
		if derived := deriveURNFromPageURL(c.pageURL); derived != "" {
			return derived, nil
		}
	}
	if c.accessToken == "" {
		return "", fmt.Errorf("linkedin access token and author URN are empty")
	}
	return c.fetchAuthenticatedPersonURN()
}

func (c *linkedinClient) fetchAuthenticatedPersonURN() (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.meEndpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set(linkedinRestliHeader, linkedinRestliVersion)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bodyBytes, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		return "", readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("linkedin /v2/me returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	var profile struct {
		ID string `json:"id"`
	}
	if unmarshalErr := json.Unmarshal(bodyBytes, &profile); unmarshalErr != nil || profile.ID == "" {
		return "", fmt.Errorf("unable to parse linkedin profile id")
	}
	c.authorURN = linkedinPersonURNPrefix + profile.ID
	return c.authorURN, nil
}

func (c *linkedinClient) shareJobPost(commentary, jobURL, title, description string) error {
	if c.accessToken == "" {
		return fmt.Errorf("linkedin access token is not configured")
	}
	authorURN, err := c.resolveAuthorURN()
	if err != nil {
		return err
	}
	payloadStruct := linkedinUGCPostRequest{
		Author:         authorURN,
		LifecycleState: "PUBLISHED",
		SpecificContent: map[string]linkedinShareContent{
			"com.linkedin.ugc.ShareContent": {
				ShareCommentary:    linkedinTextValue{Text: commentary},
				ShareMediaCategory: "ARTICLE",
				Media: []linkedinShareMedia{
					{
						Status:      "READY",
						Description: &linkedinTextValue{Text: description},
						OriginalURL: jobURL,
						Title:       &linkedinTextValue{Text: title},
					},
				},
			},
		},
		Visibility: map[string]string{
			"com.linkedin.ugc.MemberNetworkVisibility": "PUBLIC",
		},
	}
	payload, marshalErr := json.Marshal(payloadStruct)
	if marshalErr != nil {
		return marshalErr
	}
	req, reqErr := http.NewRequest(http.MethodPost, c.ugcEndpoint, bytes.NewReader(payload))
	if reqErr != nil {
		return reqErr
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(linkedinRestliHeader, linkedinRestliVersion)

	resp, doErr := c.httpClient.Do(req)
	if doErr != nil {
		return doErr
	}
	defer resp.Body.Close()
	bodyBytes, readErr := ioutil.ReadAll(resp.Body)
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("linkedin ugcPosts api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	return nil
}

func resolveLinkedinBatchLimit(cfgLinkedinJobs, cfgTwitterJobs int) int {
	if cfgLinkedinJobs > 0 {
		return cfgLinkedinJobs
	}
	if cfgTwitterJobs > 0 {
		return cfgTwitterJobs
	}
	return defaultLinkedinPostsLimit
}

func runLinkedinJobsScheduler(
	svr server.Server,
	store linkedinJobStore,
	client *linkedinClient,
) {
	lastLinkedinJobIDStr, err := store.GetValue(lastLinkedinJobIDMetaKey)
	if err != nil || lastLinkedinJobIDStr == "" {
		lastLinkedinJobIDStr = "0"
	}
	lastLinkedinJobID, convErr := strconv.Atoi(lastLinkedinJobIDStr)
	if convErr != nil {
		svr.Log(convErr, "unable to convert linkedin job str to id")
		return
	}
	cfg := svr.GetConfig()
	jobsToPost := resolveLinkedinBatchLimit(cfg.LinkedinJobsToPost, cfg.TwitterJobsToPost)
	jobPosts, fetchErr := store.GetLastNJobsFromID(jobsToPost, lastLinkedinJobID)
	if fetchErr != nil {
		svr.Log(fetchErr, "unable to retrieve jobs for linkedin scheduler")
		return
	}
	log.Printf("found %d/%d jobs to post on linkedin\n", len(jobPosts), jobsToPost)
	if len(jobPosts) == 0 {
		return
	}
	lastJobID := lastLinkedinJobID
	posted := 0
	for _, j := range jobPosts {
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
		title := fmt.Sprintf("%s with %s", j.JobTitle, j.Company)
		description := fmt.Sprintf("%s - %s | %s", j.JobTitle, j.Location, j.SalaryRange)
		if postErr := client.shareJobPost(commentary, jobURL, title, description); postErr != nil {
			svr.Log(postErr, "unable to post on linkedin")
			continue
		}
		lastJobID = j.ID
		posted++
	}
	if posted == 0 {
		return
	}
	lastJobIDStr := strconv.Itoa(lastJobID)
	if saveErr := store.SetValue(lastLinkedinJobIDMetaKey, lastJobIDStr); saveErr != nil {
		svr.Log(saveErr, fmt.Sprintf("unable to save last linkedin job id to db as %s", lastJobIDStr))
		return
	}
	log.Printf("updated last linkedin job id to %s\n", lastJobIDStr)
	log.Printf("posted last %d jobs to linkedin", posted)
}

func TriggerLinkedinJobsScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				cfg := svr.GetConfig()
				pageURL := cfg.LinkedinPageURL
				if pageURL == "" && cfg.SiteLinkedin != "" {
					pageURL = cfg.SiteLinkedin
				}
				client := newLinkedinClient(
					cfg.LinkedinAccessToken,
					cfg.LinkedinAuthorURN,
					pageURL,
				)
				runLinkedinJobsScheduler(svr, jobRepo, client)
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}
