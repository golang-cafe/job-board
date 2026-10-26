package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

const (
	twitterV2TweetsEndpoint  = "https://api.twitter.com/2/tweets"
	defaultTwitterJobsPerDay = 2
	lastTwittedJobIDKey      = "last_twitted_job_id"
)

type twitterJobRepository interface {
	GetValue(key string) (string, error)
	SetValue(key, val string) error
	GetLastNJobsFromID(max, jobID int) ([]*job.JobPost, error)
}

type twitterCreateTweetRequest struct {
	Text string `json:"text"`
}

type twitterCreateTweetResponse struct {
	Data struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"data"`
}

type twitterV2Client struct {
	endpoint          string
	httpClient        *http.Client
	clientKey         string
	clientSecret      string
	accessToken       string
	accessTokenSecret string
	nowFn             func() time.Time
	nonceFn           func() (string, error)
}

func newTwitterV2Client(cfg config.Config) *twitterV2Client {
	return &twitterV2Client{
		endpoint:          twitterV2TweetsEndpoint,
		httpClient:        &http.Client{Timeout: 15 * time.Second},
		clientKey:         cfg.TwitterClientKey,
		clientSecret:      cfg.TwitterClientSecret,
		accessToken:       cfg.TwitterAccessToken,
		accessTokenSecret: cfg.TwitterAccessTokenSecret,
		nowFn:             func() time.Time { return time.Now().UTC() },
		nonceFn:           generateOAuthNonce,
	}
}

func (c *twitterV2Client) postTweet(text string) error {
	payload, err := json.Marshal(twitterCreateTweetRequest{Text: text})
	if err != nil {
		return fmt.Errorf("unable to marshal tweet payload: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("unable to create twitter request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	authHeader, err := c.authorizationHeader(http.MethodPost, c.endpoint)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", authHeader)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to execute twitter request: %v", err)
	}
	defer func() {
		_ = res.Body.Close()
	}()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("unable to read twitter response body: %v", err)
	}
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("twitter api returned status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var tweetRes twitterCreateTweetResponse
	if err := json.Unmarshal(body, &tweetRes); err != nil {
		return fmt.Errorf("unable to decode twitter response: %v", err)
	}
	if tweetRes.Data.ID == "" {
		return fmt.Errorf("twitter api response missing tweet id")
	}
	return nil
}

func (c *twitterV2Client) authorizationHeader(method, rawURL string) (string, error) {
	if c.clientKey != "" && c.clientSecret != "" && c.accessToken != "" && c.accessTokenSecret != "" {
		nonce, err := c.nonceFn()
		if err != nil {
			return "", err
		}
		timestamp := strconv.FormatInt(c.nowFn().Unix(), 10)
		return buildOAuth1Header(
			method,
			rawURL,
			c.clientKey,
			c.clientSecret,
			c.accessToken,
			c.accessTokenSecret,
			nonce,
			timestamp,
		)
	}
	if c.accessToken != "" {
		return "Bearer " + c.accessToken, nil
	}
	return "", fmt.Errorf("missing twitter credentials")
}

func generateOAuthNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("unable to generate oauth nonce: %v", err)
	}
	return hex.EncodeToString(buf), nil
}

func buildOAuth1Header(method, rawURL, consumerKey, consumerSecret, token, tokenSecret, nonce, timestamp string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid twitter endpoint url: %v", err)
	}
	baseURL := fmt.Sprintf("%s://%s%s", strings.ToLower(parsedURL.Scheme), strings.ToLower(parsedURL.Host), parsedURL.Path)
	oauthParams := map[string]string{
		"oauth_consumer_key":     consumerKey,
		"oauth_nonce":            nonce,
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        timestamp,
		"oauth_token":            token,
		"oauth_version":          "1.0",
	}

	keys := make([]string, 0, len(oauthParams))
	for k := range oauthParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	paramPairs := make([]string, 0, len(keys))
	for _, k := range keys {
		paramPairs = append(paramPairs, fmt.Sprintf("%s=%s", percentEncode(k), percentEncode(oauthParams[k])))
	}
	parameterString := strings.Join(paramPairs, "&")
	signatureBase := strings.Join([]string{
		strings.ToUpper(method),
		percentEncode(baseURL),
		percentEncode(parameterString),
	}, "&")

	signingKey := fmt.Sprintf("%s&%s", percentEncode(consumerSecret), percentEncode(tokenSecret))
	mac := hmac.New(sha1.New, []byte(signingKey))
	if _, err := mac.Write([]byte(signatureBase)); err != nil {
		return "", fmt.Errorf("unable to sign oauth base string: %v", err)
	}
	oauthParams["oauth_signature"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))

	headerKeys := make([]string, 0, len(oauthParams))
	for k := range oauthParams {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)

	headerParts := make([]string, 0, len(headerKeys))
	for _, k := range headerKeys {
		headerParts = append(headerParts, fmt.Sprintf(`%s="%s"`, percentEncode(k), percentEncode(oauthParams[k])))
	}
	return "OAuth " + strings.Join(headerParts, ", "), nil
}

func percentEncode(input string) string {
	encoded := url.QueryEscape(input)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func isWeekday(t time.Time) bool {
	weekday := t.UTC().Weekday()
	return weekday != time.Saturday && weekday != time.Sunday
}

func twitterJobsPerDay(configured int) int {
	if configured <= 0 {
		return defaultTwitterJobsPerDay
	}
	return configured
}

func formatJobTweet(j *job.JobPost, cfg config.Config) string {
	return fmt.Sprintf(
		"%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s%s/job/%s",
		j.JobTitle,
		j.Company,
		j.Location,
		j.SalaryRange,
		cfg.SiteJobCategory,
		cfg.SiteJobCategory,
		cfg.URLProtocol,
		cfg.SiteHost,
		j.Slug,
	)
}

func runTwitterJobsScheduler(
	cfg config.Config,
	jobRepo twitterJobRepository,
	poster func(text string) error,
	logErr func(err error, msg string),
	now time.Time,
) {
	if !isWeekday(now) {
		log.Printf("skipping twitter jobs scheduler on weekend (%s)\n", now.UTC().Weekday())
		return
	}
	lastTwittedJobIDStr, err := jobRepo.GetValue(lastTwittedJobIDKey)
	if err != nil {
		logErr(err, "unable to retrieve last twitter job id")
		return
	}
	lastTwittedJobID, err := strconv.Atoi(lastTwittedJobIDStr)
	if err != nil {
		logErr(err, "unable to convert job str to id")
		return
	}
	jobsToPost := twitterJobsPerDay(cfg.TwitterJobsToPost)
	jobPosts, err := jobRepo.GetLastNJobsFromID(jobsToPost, lastTwittedJobID)
	if err != nil {
		logErr(err, "unable to retrieve last n jobs for twitter")
		return
	}
	log.Printf("found %d/%d jobs to post on twitter\n", len(jobPosts), jobsToPost)
	if len(jobPosts) == 0 {
		return
	}
	lastJobID := lastTwittedJobID
	postedCount := 0
	for _, j := range jobPosts {
		if err := poster(formatJobTweet(j, cfg)); err != nil {
			logErr(err, "unable to post tweet")
			continue
		}
		lastJobID = j.ID
		postedCount++
	}
	if lastJobID == lastTwittedJobID {
		return
	}
	lastJobIDStr := strconv.Itoa(lastJobID)
	if err := jobRepo.SetValue(lastTwittedJobIDKey, lastJobIDStr); err != nil {
		logErr(err, fmt.Sprintf("unable to save last twitter job id to db as %s", lastJobIDStr))
		return
	}
	log.Printf("updated last twitted job id to %s\n", lastJobIDStr)
	log.Printf("posted last %d jobs to twitter", postedCount)
}

// TriggerTwitterJobsScheduler triggers the weekday Twitter API v2 job sharing task.
func TriggerTwitterJobsScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				cfg := svr.GetConfig()
				client := newTwitterV2Client(cfg)
				runTwitterJobsScheduler(cfg, jobRepo, client.postTweet, svr.Log, time.Now().UTC())
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}
