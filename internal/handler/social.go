package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/golang-cafe/job-board/internal/job"
)

const (
	twitterTweetEndpoint    = "https://api.twitter.com/2/tweets"
	linkedinUGCPostEndpoint = "https://api.linkedin.com/v2/ugcPosts"
)

type twitterClient struct {
	accessToken       string
	accessTokenSecret string
	clientKey         string
	clientSecret      string
	endpoint          string
	httpClient        *http.Client
	nonce             func() string
	now               func() time.Time
}

type linkedinClient struct {
	accessToken string
	authorURN   string
	endpoint    string
	httpClient  *http.Client
}

func formatSocialJobPost(j *job.JobPost, category, protocol, host string) string {
	return fmt.Sprintf("%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s%s/job/%s", j.JobTitle, j.Company, j.Location, j.SalaryRange, category, category, protocol, host, j.Slug)
}

func isWeekday(now time.Time) bool {
	weekday := now.Weekday()
	return weekday != time.Saturday && weekday != time.Sunday
}

func (c twitterClient) postTweet(ctx context.Context, text string) error {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", c.authorizationHeader(http.MethodPost, c.endpoint))

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("twitter API returned %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c twitterClient) authorizationHeader(method, rawURL string) string {
	nonce := randomNonce
	if c.nonce != nil {
		nonce = c.nonce
	}
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	params := map[string]string{
		"oauth_consumer_key":     c.clientKey,
		"oauth_nonce":            nonce(),
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        fmt.Sprintf("%d", now().Unix()),
		"oauth_token":            c.accessToken,
		"oauth_version":          "1.0",
	}
	params["oauth_signature"] = c.oauthSignature(method, rawURL, params)

	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, oauthEscape(key), oauthEscape(params[key])))
	}
	return "OAuth " + strings.Join(parts, ", ")
}

func (c twitterClient) oauthSignature(method, rawURL string, params map[string]string) string {
	baseURL := rawURL
	if parsed, err := url.Parse(rawURL); err == nil {
		baseURL = parsed.Scheme + "://" + parsed.Host + parsed.Path
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	encoded := make([]string, 0, len(keys))
	for _, key := range keys {
		encoded = append(encoded, oauthEscape(key)+"="+oauthEscape(params[key]))
	}
	base := strings.ToUpper(method) + "&" + oauthEscape(baseURL) + "&" + oauthEscape(strings.Join(encoded, "&"))
	signingKey := oauthEscape(c.clientSecret) + "&" + oauthEscape(c.accessTokenSecret)
	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(base))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (c linkedinClient) shareArticle(ctx context.Context, text, title, link string) error {
	payload := map[string]interface{}{
		"author":         c.authorURN,
		"lifecycleState": "PUBLISHED",
		"specificContent": map[string]interface{}{
			"com.linkedin.ugc.ShareContent": map[string]interface{}{
				"shareCommentary":    map[string]string{"text": text},
				"shareMediaCategory": "ARTICLE",
				"media": []map[string]interface{}{
					{
						"status":      "READY",
						"originalUrl": link,
						"title":       map[string]string{"text": title},
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("linkedin API returned %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func oauthEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func randomNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
