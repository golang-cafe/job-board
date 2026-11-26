// Package linkedin posts approved jobs to LinkedIn using the
// LinkedIn "Share on LinkedIn" User Generated Content (UGC) API:
//
//	POST https://api.linkedin.com/v2/ugcPosts
//
// See https://learn.microsoft.com/en-us/linkedin/consumer/integrations/self-serve/share-on-linkedin
//
// Posting requires an access token issued with the `w_member_social` scope.
// The token belongs to a member (for a personal profile post) or to an
// admin of a company page (for an organization post); the author of each
// share is identified by its LinkedIn URN, e.g. `urn:li:person:8675309`
// or `urn:li:organization:1234567`.
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
)

// ugcPostsURL is the LinkedIn UGC API endpoint used to create shares.
// It is a variable (not a constant) so tests can point the client at a
// local test server.
var ugcPostsURL = "https://api.linkedin.com/v2/ugcPosts"

// Client posts shares to LinkedIn on behalf of one author URN.
type Client struct {
	httpClient *http.Client
	token      string
	authorURN  string
}

// NewClient returns a Client that authenticates with the given access
// token and posts as the given author URN.
func NewClient(token, authorURN string) *Client {
	return &Client{
		httpClient: http.DefaultClient,
		token:      token,
		authorURN:  authorURN,
	}
}

// NewClientWithHTTPClient is NewClient with an injectable HTTP client,
// used by tests.
func NewClientWithHTTPClient(httpClient *http.Client, token, authorURN string) *Client {
	return &Client{
		httpClient: httpClient,
		token:      token,
		authorURN:  authorURN,
	}
}

// textBlock is a LinkedIn text node, e.g. share commentary, title or
// description.
type textBlock struct {
	Text string `json:"text"`
}

// shareMedia is one media attachment of a share.
type shareMedia struct {
	Status      string     `json:"status"`
	Description *textBlock `json:"description,omitempty"`
	Media       string     `json:"media,omitempty"`
	OriginalURL string     `json:"originalUrl,omitempty"`
	Title       *textBlock `json:"title,omitempty"`
}

// shareContent is the com.linkedin.ugc.ShareContent object.
type shareContent struct {
	ShareCommentary    textBlock    `json:"shareCommentary"`
	ShareMediaCategory string       `json:"shareMediaCategory"`
	Media              []shareMedia `json:"media,omitempty"`
}

// ugcPost is the request body for POST /v2/ugcPosts.
type ugcPost struct {
	Author          string `json:"author"`
	LifecycleState  string `json:"lifecycleState"`
	SpecificContent struct {
		ShareContent shareContent `json:"com.linkedin.ugc.ShareContent"`
	} `json:"specificContent"`
	Visibility struct {
		Visibility string `json:"com.linkedin.ugc.MemberNetworkVisibility"`
	} `json:"visibility"`
}

// BuildArticleSharePayload builds the UGC API request body for an article
// (URL) share: the commentary text plus a link card with the given title
// and description.
func BuildArticleSharePayload(authorURN, commentary, articleURL, title, description string) ugcPost {
	post := ugcPost{
		Author:         authorURN,
		LifecycleState: "PUBLISHED",
	}
	post.SpecificContent.ShareContent = shareContent{
		ShareCommentary:    textBlock{Text: commentary},
		ShareMediaCategory: "ARTICLE",
		Media: []shareMedia{
			{
				Status:      "READY",
				Description: &textBlock{Text: description},
				OriginalURL: articleURL,
				Title:       &textBlock{Text: title},
			},
		},
	}
	post.Visibility.Visibility = "PUBLIC"
	return post
}

// ShareArticle publishes one article share and returns the LinkedIn URN
// of the created post (the X-RestLi-Id response header). A non-201
// response is returned as an error.
func (c *Client) ShareArticle(ctx context.Context, commentary, articleURL, title, description string) (string, error) {
	payload, err := json.Marshal(BuildArticleSharePayload(c.authorURN, commentary, articleURL, title, description))
	if err != nil {
		return "", fmt.Errorf("unable to encode linkedin share payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ugcPostsURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("unable to create linkedin share request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("unable to post share to linkedin: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := ioutil.ReadAll(resp.Body)
		return "", fmt.Errorf("linkedin share failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Header.Get("X-RestLi-Id"), nil
}

// ResolveAuthorURN returns the author URN a share should be posted as.
//
// LINKEDIN_AUTHOR_URN wins when set. Otherwise LINKEDIN_PAGE_URL is
// resolved: a raw URN is passed through, a company page URL
// (https://www.linkedin.com/company/<id>/) resolves to
// urn:li:organization:<id> when <id> is numeric, and a personal profile
// URL (https://www.linkedin.com/in/<id>/) resolves to
// urn:li:person:<id>. A company vanity name cannot be resolved to a URN
// without an extra API call, so it is rejected with a hint to set
// LINKEDIN_AUTHOR_URN to the numeric organization URN instead.
func ResolveAuthorURN(pageURL, authorURN string) (string, error) {
	if strings.TrimSpace(authorURN) != "" {
		authorURN = strings.TrimSpace(authorURN)
		if !strings.HasPrefix(authorURN, "urn:li:") {
			return "", fmt.Errorf("LINKEDIN_AUTHOR_URN must be a LinkedIn URN such as urn:li:person:8675309 or urn:li:organization:1234567, got %q", authorURN)
		}
		return authorURN, nil
	}
	pageURL = strings.TrimSpace(pageURL)
	if pageURL == "" {
		return "", fmt.Errorf("no linkedin author configured: set LINKEDIN_AUTHOR_URN or LINKEDIN_PAGE_URL")
	}
	if strings.HasPrefix(pageURL, "urn:li:") {
		return pageURL, nil
	}
	u, err := url.Parse(pageURL)
	if err != nil {
		return "", fmt.Errorf("unable to parse LINKEDIN_PAGE_URL %q: %w", pageURL, err)
	}
	if !strings.Contains(strings.ToLower(u.Host), "linkedin.com") {
		return "", fmt.Errorf("LINKEDIN_PAGE_URL must be a linkedin.com URL or a urn:li: URN, got %q", pageURL)
	}
	parts := strings.FieldsFunc(strings.Trim(u.Path, "/"), func(r rune) bool { return r == '/' })
	if len(parts) < 2 {
		return "", fmt.Errorf("unable to resolve author URN from LINKEDIN_PAGE_URL %q: expected a /company/<id> or /in/<id> URL, or set LINKEDIN_AUTHOR_URN directly", pageURL)
	}
	kind, id := strings.ToLower(parts[0]), parts[1]
	switch kind {
	case "company":
		if _, err := strconv.Atoi(id); err != nil {
			return "", fmt.Errorf("LINKEDIN_PAGE_URL %q uses a company vanity name %q, which cannot be resolved to a URN: set LINKEDIN_AUTHOR_URN to urn:li:organization:<numeric id>", pageURL, id)
		}
		return "urn:li:organization:" + id, nil
	case "in":
		return "urn:li:person:" + id, nil
	default:
		return "", fmt.Errorf("unable to resolve author URN from LINKEDIN_PAGE_URL %q: expected a /company/<id> or /in/<id> URL, or set LINKEDIN_AUTHOR_URN directly", pageURL)
	}
}
