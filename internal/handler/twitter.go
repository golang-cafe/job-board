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

	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

const (
	defaultTwitterV2Endpoint = "https://api.twitter.com/2/tweets"
	defaultDailyTweetsLimit  = 2
)

type twitterJobStore interface {
	GetValue(key string) (string, error)
	SetValue(key, val string) error
	GetLastNJobsFromID(max, jobID int) ([]job.JobPost, error)
}

type twitterV2Client struct {
	endpoint          string
	consumerKey       string
	consumerSecret    string
	accessToken       string
	accessTokenSecret string
	httpClient        *http.Client
	nowFunc           func() time.Time
}

func newTwitterV2Client(consumerKey, consumerSecret, accessToken, accessTokenSecret string) *twitterV2Client {
	return &twitterV2Client{
		endpoint:          defaultTwitterV2Endpoint,
		consumerKey:       consumerKey,
		consumerSecret:    consumerSecret,
		accessToken:       accessToken,
		accessTokenSecret: accessTokenSecret,
		httpClient:        &http.Client{Timeout: 15 * time.Second},
		nowFunc:           time.Now,
	}
}

func (c *twitterV2Client) postTweet(text string) error {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	authHeader, err := c.buildOAuth1Header(http.MethodPost, c.endpoint)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := ioutil.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("twitter v2 api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	return nil
}

func (c *twitterV2Client) buildOAuth1Header(method, rawURL string) (string, error) {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", err
	}
	nonce := hex.EncodeToString(nonceBytes)
	timestamp := strconv.FormatInt(c.nowFunc().UTC().Unix(), 10)

	params := map[string]string{
		"oauth_consumer_key":     c.consumerKey,
		"oauth_nonce":            nonce,
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        timestamp,
		"oauth_token":            c.accessToken,
		"oauth_version":          "1.0",
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	baseURL := fmt.Sprintf("%s://%s%s", parsedURL.Scheme, parsedURL.Host, parsedURL.Path)

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%s", percentEncode(k), percentEncode(params[k])))
	}
	paramString := strings.Join(pairs, "&")
	signatureBase := fmt.Sprintf("%s&%s&%s", strings.ToUpper(method), percentEncode(baseURL), percentEncode(paramString))
	signingKey := fmt.Sprintf("%s&%s", percentEncode(c.consumerSecret), percentEncode(c.accessTokenSecret))

	mac := hmac.New(sha1.New, []byte(signingKey))
	_, _ = mac.Write([]byte(signatureBase))
	params["oauth_signature"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))

	headerKeys := make([]string, 0, len(params))
	for k := range params {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)

	headerParts := make([]string, 0, len(headerKeys))
	for _, k := range headerKeys {
		headerParts = append(headerParts, fmt.Sprintf(`%s="%s"`, percentEncode(k), percentEncode(params[k])))
	}
	return "OAuth " + strings.Join(headerParts, ", "), nil
}

func percentEncode(s string) string {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func isWeekday(t time.Time) bool {
	wd := t.UTC().Weekday()
	return wd != time.Saturday && wd != time.Sunday
}

func runTwitterJobsScheduler(
	svr server.Server,
	store twitterJobStore,
	client *twitterV2Client,
	now time.Time,
) {
	if !isWeekday(now) {
		log.Printf("skipping twitter scheduler on weekend (%s)\n", now.UTC().Weekday())
		return
	}
	lastTwittedJobIDStr, err := store.GetValue("last_twitted_job_id")
	if err != nil {
		svr.Log(err, "unable to retrieve last twitter job id")
		return
	}
	lastTwittedJobID, err := strconv.Atoi(lastTwittedJobIDStr)
	if err != nil {
		svr.Log(err, "unable to convert job str to id")
		return
	}
	jobsToPost := svr.GetConfig().TwitterJobsToPost
	if jobsToPost <= 0 || jobsToPost > defaultDailyTweetsLimit {
		jobsToPost = defaultDailyTweetsLimit
	}
	jobPosts, err := store.GetLastNJobsFromID(jobsToPost, lastTwittedJobID)
	if err != nil {
		svr.Log(err, "unable to retrieve jobs for twitter scheduler")
		return
	}
	log.Printf("found %d/%d jobs to post on twitter\n", len(jobPosts), jobsToPost)
	if len(jobPosts) == 0 {
		return
	}
	lastJobID := lastTwittedJobID
	posted := 0
	cfg := svr.GetConfig()
	for _, j := range jobPosts {
		text := fmt.Sprintf(
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
		if err := client.postTweet(text); err != nil {
			svr.Log(err, "unable to post tweet")
			continue
		}
		lastJobID = j.ID
		posted++
	}
	if posted == 0 {
		return
	}
	lastJobIDStr := strconv.Itoa(lastJobID)
	if err := store.SetValue("last_twitted_job_id", lastJobIDStr); err != nil {
		svr.Log(err, fmt.Sprintf("unable to save last twitter job id to db as %s", lastJobIDStr))
		return
	}
	log.Printf("updated last twitted job id to %s\n", lastJobIDStr)
	log.Printf("posted last %d jobs to twitter", posted)
}

func TriggerTwitterJobsScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				cfg := svr.GetConfig()
				client := newTwitterV2Client(
					cfg.TwitterClientKey,
					cfg.TwitterClientSecret,
					cfg.TwitterAccessToken,
					cfg.TwitterAccessTokenSecret,
				)
				runTwitterJobsScheduler(svr, jobRepo, client, time.Now())
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}
