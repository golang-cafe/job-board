package twitter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/garyburd/go-oauth/oauth"
)

type Client struct {
	consumer oauth.Client
	user     oauth.Credentials
	http     *http.Client
	endpoint string
}

func NewClient(key, secret, token, tokenSecret string) (*Client, error) {
	if key == "" || secret == "" || token == "" || tokenSecret == "" {
		return nil, errors.New("X user-context credentials are not configured")
	}
	return &Client{
		consumer: oauth.Client{Credentials: oauth.Credentials{Token: key, Secret: secret}},
		user:     oauth.Credentials{Token: token, Secret: tokenSecret},
		http:     &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		endpoint: "https://api.x.com/2/tweets",
	}, nil
}

func (c *Client) Post(ctx context.Context, text string) (string, error) {
	body, err := json.Marshal(struct {
		Text string `json:"text"`
	}{text})
	if err != nil {
		return "", err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	r.Header.Set("Content-Type", "application/json")
	if err = c.consumer.SetAuthorizationHeader(r.Header, &c.user, r.Method, r.URL, nil); err != nil {
		return "", errors.New("unable to authorize X request")
	}
	res, err := c.http.Do(r)
	if err != nil {
		return "", errors.New("X request failed; delivery may be uncertain")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("X create-post returned HTTP %d", res.StatusCode)
	}
	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 64*1024)).Decode(&result); err != nil || result.Data.ID == "" {
		return "", errors.New("X create-post returned an invalid acknowledgement; delivery may be uncertain")
	}
	return result.Data.ID, nil
}
