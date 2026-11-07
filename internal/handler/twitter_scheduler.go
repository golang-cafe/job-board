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
	"io"
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
	twitterV2TweetsEndpoint = "https://api.twitter.com/2/tweets"
	defaultDailyTweetLimit  = 2
)

// twitterV2Client encapsulates OAuth 1.0a credentials and HTTP transport for the Twitter v2 Manage Tweets API.
type twitterV2Client struct {
	consumerKey    string
	consumerSecret string
	accessToken    string
	tokenSecret    string
	endpoint       string
	httpClient     *http.Client
}

// oauthRequestParams groups the HTTP method, target URL, and timestamp/nonce state required to sign an OAuth 1.0a request.
type oauthRequestParams struct {
	method    string
	rawURL    string
	nonce     string
	timestamp string
}

// twitterBatchRunner coordinates job retrieval, Twitter v2 publication, and cursor persistence for the daily scheduler.
type twitterBatchRunner struct {
	svr     server.Server
	jobRepo *job.Repository
	client  *twitterV2Client
}

// newTwitterV2Client constructs a Twitter v2 API client from application configuration.
func newTwitterV2Client(cfg config.Config) *twitterV2Client {
	return &twitterV2Client{
		consumerKey:    cfg.TwitterClientKey,
		consumerSecret: cfg.TwitterClientSecret,
		accessToken:    cfg.TwitterAccessToken,
		tokenSecret:    cfg.TwitterAccessTokenSecret,
		endpoint:       twitterV2TweetsEndpoint,
		httpClient:     &http.Client{Timeout: 15 * time.Second},
	}
}

// isWeekday returns true strictly from Monday through Friday so scheduled job tweets never fire on weekends.
func isWeekday(t time.Time) bool {
	day := t.UTC().Weekday()
	return day != time.Saturday && day != time.Sunday
}

// resolveDailyTweetLimit caps the number of jobs shared per weekday execution to 2 as required by the daily schedule.
func resolveDailyTweetLimit(configured int) int {
	if configured <= 0 || configured > defaultDailyTweetLimit {
		return defaultDailyTweetLimit
	}
	return configured
}

// formatJobTweet builds the canonical job announcement copy with category hashtags and direct job permalink.
func formatJobTweet(cfg config.Config, j job.JobPost) string {
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

// TriggerTwitterJobsScheduler handles /x/task/twitter-jobs-scheduler and shares up to 2 jobs per weekday via Twitter API v2.
func TriggerTwitterJobsScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	runner := &twitterBatchRunner{
		svr:     svr,
		jobRepo: jobRepo,
		client:  newTwitterV2Client(svr.GetConfig()),
	}
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go runner.runBatch()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

// runBatch executes the asynchronous weekday job lookup, Twitter v2 publication, and cursor persistence.
func (r *twitterBatchRunner) runBatch() {
	if !isWeekday(time.Now().UTC()) {
		log.Printf("skipping twitter jobs scheduler on weekend\n")
		return
	}
	lastJobID, err := loadLastTwittedJobID(r.jobRepo)
	if err != nil {
		r.svr.Log(err, "unable to retrieve last twitter job id")
		return
	}
	limit := resolveDailyTweetLimit(r.svr.GetConfig().TwitterJobsToPost)
	jobPosts, err := r.jobRepo.GetLastNJobsFromID(limit, lastJobID)
	if err != nil {
		r.svr.Log(err, "unable to fetch jobs for twitter scheduler")
		return
	}
	log.Printf("found %d/%d jobs to post on twitter\n", len(jobPosts), limit)
	if len(jobPosts) == 0 {
		return
	}
	updatedID := r.publishJobs(jobPosts, lastJobID)
	r.saveLastJobID(updatedID, len(jobPosts))
}

func loadLastTwittedJobID(jobRepo *job.Repository) (int, error) {
	rawID, err := jobRepo.GetValue("last_twitted_job_id")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(rawID)
}

func (r *twitterBatchRunner) publishJobs(jobs []job.JobPost, initialID int) int {
	lastID := initialID
	for _, j := range jobs {
		tweetText := formatJobTweet(r.svr.GetConfig(), j)
		if err := r.client.postTweet(tweetText); err != nil {
			r.svr.Log(err, "unable to post tweet via twitter v2 api")
			continue
		}
		lastID = j.ID
	}
	return lastID
}

func (r *twitterBatchRunner) saveLastJobID(lastID int, count int) {
	lastJobIDStr := strconv.Itoa(lastID)
	if err := r.jobRepo.SetValue("last_twitted_job_id", lastJobIDStr); err != nil {
		r.svr.Log(err, fmt.Sprintf("unable to save last twitter job id to db as %s", lastJobIDStr))
		return
	}
	log.Printf("updated last twitted job id to %s\n", lastJobIDStr)
	log.Printf("posted last %d jobs to twitter", count)
}

// postTweet publishes a status update through the Twitter v2 Manage Tweets endpoint (POST /2/tweets).
func (c *twitterV2Client) postTweet(text string) error {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	params := oauthRequestParams{
		method:    http.MethodPost,
		rawURL:    c.endpoint,
		nonce:     generateOAuthNonce(),
		timestamp: strconv.FormatInt(time.Now().Unix(), 10),
	}
	req.Header.Set("Authorization", c.buildOAuthHeader(params))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return validateTwitterResponse(resp)
}

func validateTwitterResponse(resp *http.Response) error {
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("twitter api v2 returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
}

// buildOAuthHeader constructs the RFC 5849 OAuth 1.0a Authorization header for Twitter v2 JSON requests.
func (c *twitterV2Client) buildOAuthHeader(p oauthRequestParams) string {
	oauthPairs := map[string]string{
		"oauth_consumer_key":     c.consumerKey,
		"oauth_nonce":            p.nonce,
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        p.timestamp,
		"oauth_token":            c.accessToken,
		"oauth_version":          "1.0",
	}
	signature := c.computeOAuthSignature(p, oauthPairs)
	oauthPairs["oauth_signature"] = signature
	keys := sortedKeys(oauthPairs)
	headerParts := make([]string, 0, len(keys))
	for _, k := range keys {
		headerParts = append(headerParts, fmt.Sprintf(`%s="%s"`, percentEncode(k), percentEncode(oauthPairs[k])))
	}
	return "OAuth " + strings.Join(headerParts, ", ")
}

func (c *twitterV2Client) computeOAuthSignature(p oauthRequestParams, oauthPairs map[string]string) string {
	keys := sortedKeys(oauthPairs)
	encodedPairs := make([]string, 0, len(keys))
	for _, k := range keys {
		encodedPairs = append(encodedPairs, percentEncode(k)+"="+percentEncode(oauthPairs[k]))
	}
	paramString := strings.Join(encodedPairs, "&")
	baseString := strings.ToUpper(p.method) + "&" + percentEncode(p.rawURL) + "&" + percentEncode(paramString)
	signingKey := percentEncode(c.consumerSecret) + "&" + percentEncode(c.tokenSecret)
	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(baseString))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func percentEncode(input string) string {
	encoded := url.QueryEscape(input)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func generateOAuthNonce() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(buf)
}
