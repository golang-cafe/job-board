package twitter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dghubble/oauth1"
)

const tweetsURL = "https://api.twitter.com/2/tweets"

// Client posts tweets via Twitter API v2 using OAuth 1.0a user context.
type Client struct {
	httpClient *http.Client
}

func NewClient(apiKey, apiSecret, accessToken, accessSecret string) *Client {
	config := oauth1.NewConfig(apiKey, apiSecret)
	token := oauth1.NewToken(accessToken, accessSecret)
	return &Client{httpClient: config.Client(oauth1.NoContext, token)}
}

type createTweetRequest struct {
	Text string `json:"text"`
}

type createTweetError struct {
	Detail string `json:"detail"`
	Title  string `json:"title"`
}

type createTweetResponse struct {
	Data struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"data"`
	Errors []createTweetError `json:"errors"`
	Title  string             `json:"title"`
	Detail string             `json:"detail"`
}

// PostTweet creates a tweet. Returns the new tweet id.
func (c *Client) PostTweet(text string) (string, error) {
	body, err := json.Marshal(createTweetRequest{Text: text})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, tweetsURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed createTweetResponse
	_ = json.Unmarshal(raw, &parsed)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := parsed.Detail
		if msg == "" && len(parsed.Errors) > 0 {
			msg = parsed.Errors[0].Detail
		}
		if msg == "" {
			msg = string(raw)
		}
		return "", fmt.Errorf("twitter api v2 status %d: %s", resp.StatusCode, msg)
	}
	if parsed.Data.ID == "" {
		return "", fmt.Errorf("twitter api v2 returned empty tweet id: %s", string(raw))
	}
	return parsed.Data.ID, nil
}

// IsWeekday reports whether t is Monday–Friday in the given location.
func IsWeekday(t time.Time, loc *time.Location) bool {
	if loc != nil {
		t = t.In(loc)
	}
	wd := t.Weekday()
	return wd >= time.Monday && wd <= time.Friday
}
