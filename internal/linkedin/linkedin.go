package linkedin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const DefaultLinkedInUGCEndpoint = "https://api.linkedin.com/v2/ugcPosts"

type JobPostItem struct {
	ID           int
	JobTitle     string
	Company      string
	Location     string
	SalaryRange  string
	Slug         string
	ExternalURL  string
}

type Client struct {
	AccessToken string
	AuthorURN   string
	PageURL     string
	SiteHost    string
	Endpoint    string
	HTTPClient  *http.Client
}

func NewClient(accessToken, authorURN, pageURL, siteHost string) *Client {
	return &Client{
		AccessToken: accessToken,
		AuthorURN:   authorURN,
		PageURL:     pageURL,
		SiteHost:    siteHost,
		Endpoint:    DefaultLinkedInUGCEndpoint,
		HTTPClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

func BuildUGCPostPayload(authorURN string, siteHost string, job JobPostItem) map[string]interface{} {
	jobURL := fmt.Sprintf("https://%s/job/%s", siteHost, job.Slug)
	commentary := fmt.Sprintf(
		"New job opportunity: %s at %s (%s) — %s
Apply now: %s",
		job.JobTitle,
		job.Company,
		job.Location,
		job.SalaryRange,
		jobURL,
	)
	return map[string]interface{}{
		"author":         authorURN,
		"lifecycleState": "PUBLISHED",
		"specificContent": map[string]interface{}{
			"com.linkedin.ugc.ShareContent": map[string]interface{}{
				"shareCommentary": map[string]interface{}{
					"text": commentary,
				},
				"shareMediaCategory": "ARTICLE",
				"media": []map[string]interface{}{
					{
						"status":      "READY",
						"originalUrl": jobURL,
						"title": map[string]interface{}{
							"text": fmt.Sprintf("%s at %s", job.JobTitle, job.Company),
						},
					},
				},
			},
		},
		"visibility": map[string]interface{}{
			"com.linkedin.ugc.MemberNetworkVisibility": "PUBLIC",
		},
	}
}

func (c *Client) ShareJobPost(job JobPostItem) (string, error) {
	if c.AccessToken == "" || c.AuthorURN == "" {
		return "", errors.New("linkedin access token and author URN are required")
	}
	payload := BuildUGCPostPayload(c.AuthorURN, c.SiteHost, job)
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, c.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("linkedin ugcPosts returned status %d", resp.StatusCode)
	}
	return resp.Header.Get("X-RestLi-Id"), nil
}
