// Package linkedin provides a minimal client for the LinkedIn
// "Share on LinkedIn" API (ugcPosts) used to share job posts on the
// site's LinkedIn company page, mirroring the existing Telegram and
// Twitter job sharing integrations.
package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the LinkedIn API base URL.
const DefaultBaseURL = "https://api.linkedin.com"

// Client is a LinkedIn ugcPosts API client posting on behalf of the
// author (company page) derived from the configured page URL.
type Client struct {
	AccessToken string
	Author      string // author URN, e.g. urn:li:organization:12345678
	BaseURL     string
	HTTPClient  *http.Client
}

// NewClient creates a Client from an OAuth access token and the URL of
// the LinkedIn page to post as (LINKEDIN_PAGE_URL), e.g.
// https://www.linkedin.com/company/12345678
func NewClient(accessToken, pageURL string) (*Client, error) {
	author, err := AuthorURNFromPageURL(pageURL)
	if err != nil {
		return nil, err
	}
	return &Client{
		AccessToken: accessToken,
		Author:      author,
		BaseURL:     DefaultBaseURL,
		HTTPClient:  &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// AuthorURNFromPageURL derives the ugcPosts author URN from a LinkedIn
// page URL. Company page URLs carry the numeric page id
// (https://www.linkedin.com/company/12345678) which maps to
// urn:li:organization:12345678. A ready-made URN (urn:li:...) is also
// accepted as-is.
func AuthorURNFromPageURL(pageURL string) (string, error) {
	pageURL = strings.TrimSpace(pageURL)
	if strings.HasPrefix(pageURL, "urn:li:") {
		return pageURL, nil
	}
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", fmt.Errorf("unable to parse linkedin page url %q: %v", pageURL, err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "company" {
		return "", fmt.Errorf("linkedin page url %q is not a company page url (expected https://www.linkedin.com/company/<page-id>)", pageURL)
	}
	if _, err := strconv.ParseInt(parts[1], 10, 64); err != nil {
		return "", fmt.Errorf("linkedin page url %q does not carry a numeric page id, use the page url containing the numeric id or a full author urn (urn:li:organization:<id>)", pageURL)
	}
	return "urn:li:organization:" + parts[1], nil
}

// ugcPost is the request body of POST /v2/ugcPosts.
type ugcPost struct {
	Author          string          `json:"author"`
	LifecycleState  string          `json:"lifecycleState"`
	SpecificContent specificContent `json:"specificContent"`
	Visibility      visibility      `json:"visibility"`
}

type specificContent struct {
	ShareContent shareContent `json:"com.linkedin.ugc.ShareContent"`
}

type shareContent struct {
	ShareCommentary    shareCommentary `json:"shareCommentary"`
	ShareMediaCategory string          `json:"shareMediaCategory"`
	Media              []shareMedia    `json:"media"`
}

type shareCommentary struct {
	Text string `json:"text"`
}

type shareMedia struct {
	Status      string `json:"status"`
	OriginalURL string `json:"originalUrl"`
}

type visibility struct {
	MemberNetworkVisibility string `json:"com.linkedin.ugc.MemberNetworkVisibility"`
}

// BuildSharePayload builds the ugcPosts request body sharing
// originalURL with the given commentary text as a public post.
func BuildSharePayload(author, text, originalURL string) ugcPost {
	return ugcPost{
		Author:         author,
		LifecycleState: "PUBLISHED",
		SpecificContent: specificContent{
			ShareContent: shareContent{
				ShareCommentary:    shareCommentary{Text: text},
				ShareMediaCategory: "ARTICLE",
				Media: []shareMedia{
					{Status: "READY", OriginalURL: originalURL},
				},
			},
		},
		Visibility: visibility{MemberNetworkVisibility: "PUBLIC"},
	}
}

// SharePost publishes a post with the given commentary text sharing
// originalURL on the client's author page.
func (c *Client) SharePost(ctx context.Context, text, originalURL string) error {
	body, err := json.Marshal(BuildSharePayload(c.Author, text, originalURL))
	if err != nil {
		return fmt.Errorf("unable to marshal linkedin share payload: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v2/ugcPosts", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("unable to create linkedin share request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to post on linkedin: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("unable to post on linkedin: unexpected status %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	return nil
}
