package twitterapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dghubble/oauth1"
)

const defaultEndpoint = "https://api.twitter.com/2/tweets"

type Client struct {
	httpClient *http.Client
	endpoint   string
}

func NewClient(consumerKey, consumerSecret, accessToken, accessTokenSecret string) *Client {
	config := oauth1.NewConfig(consumerKey, consumerSecret)
	token := oauth1.NewToken(accessToken, accessTokenSecret)
	return &Client{
		httpClient: config.Client(oauth1.NoContext, token),
		endpoint:   defaultEndpoint,
	}
}

func (c *Client) PostTweet(ctx context.Context, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("tweet text cannot be empty")
	}

	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return fmt.Errorf("marshal tweet: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create tweet request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post tweet: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("post tweet: twitter api returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	return nil
}
