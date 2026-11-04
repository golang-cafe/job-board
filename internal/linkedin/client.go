package linkedin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const endpoint = "https://api.linkedin.com/rest/posts"

var authorPattern = regexp.MustCompile(`^urn:li:(person|organization):[A-Za-z0-9_-]+$`)
var versionPattern = regexp.MustCompile(`^[0-9]{4}(0[1-9]|1[0-2])$`)

type Client struct {
	token, author, version string
	http                   *http.Client
}

func NewClient(token, author, version string) (*Client, error) {
	if strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") || !authorPattern.MatchString(author) || !versionPattern.MatchString(version) {
		return nil, errors.New("LinkedIn token, author URN and API version must be configured")
	}
	return &Client{token: token, author: author, version: version, http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Client) Post(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" || len([]rune(text)) > 3000 {
		return "", errors.New("invalid LinkedIn commentary length")
	}
	body, err := json.Marshal(struct {
		Author                    string                 `json:"author"`
		Commentary                string                 `json:"commentary"`
		Visibility                string                 `json:"visibility"`
		Distribution              map[string]interface{} `json:"distribution"`
		LifecycleState            string                 `json:"lifecycleState"`
		IsReshareDisabledByAuthor bool                   `json:"isReshareDisabledByAuthor"`
	}{c.author, text, "PUBLIC", map[string]interface{}{"feedDistribution": "MAIN_FEED", "targetEntities": []string{}, "thirdPartyDistributionChannels": []string{}}, "PUBLISHED", false})
	if err != nil {
		return "", errors.New("unable to encode LinkedIn post")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", errors.New("unable to create LinkedIn request")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
	req.Header.Set("Linkedin-Version", c.version)
	res, err := c.http.Do(req)
	if err != nil {
		return "", errors.New("LinkedIn transport failed; reconcile before retrying")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("LinkedIn returned status %d; reconcile before retrying", res.StatusCode)
	}
	id := res.Header.Get("X-Restli-Id")
	if !regexp.MustCompile(`^urn:li:(share|ugcPost):[0-9]+$`).MatchString(id) {
		return "", errors.New("LinkedIn success response missing valid post ID")
	}
	return id, nil
}
