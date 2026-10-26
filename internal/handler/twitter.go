package handler

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

const (
	defaultTwitterV2Endpoint = "https://api.twitter.com/2/tweets"
	defaultTwitterJobsToPost = 2
	maxTweetCharacters       = 280
	lastTwittedJobIDMetaKey  = "last_twitted_job_id"
)

type tweetV2Request struct {
	Text string `json:"text"`
}

// TriggerTwitterJobsScheduler triggers daily weekday job sharing to Twitter using the Twitter v2 API.
func TriggerTwitterJobsScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go runTwitterScheduler(svr, jobRepo, &http.Client{Timeout: 15 * time.Second}, defaultTwitterV2Endpoint, time.Now().UTC())
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func isWeekday(t time.Time) bool {
	wd := t.UTC().Weekday()
	return wd != time.Saturday && wd != time.Sunday
}

func formatTweetText(j *job.JobPost, cfg config.Config) string {
	headline := fmt.Sprintf("%s with %s - %s | %s", j.JobTitle, j.Company, j.Location, j.SalaryRange)
	suffix := fmt.Sprintf("\n\n#%s #%sjobs\n\n%s%s/job/%s", cfg.SiteJobCategory, cfg.SiteJobCategory, cfg.URLProtocol, cfg.SiteHost, j.Slug)
	full := headline + suffix
	if utf8.RuneCountInString(full) <= maxTweetCharacters {
		return full
	}
	allowedHeadlineRunes := maxTweetCharacters - utf8.RuneCountInString(suffix) - 3
	if allowedHeadlineRunes <= 0 {
		runes := []rune(full)
		return string(runes[:maxTweetCharacters])
	}
	headlineRunes := []rune(headline)
	if len(headlineRunes) > allowedHeadlineRunes {
		headline = strings.TrimSpace(string(headlineRunes[:allowedHeadlineRunes])) + "..."
	}
	return headline + suffix
}

func percentEncodeRFC3986(s string) string {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func generateOAuthNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func buildOAuth1Header(method, rawURL, consumerKey, consumerSecret, accessToken, accessSecret, nonce, timestamp string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	baseURL := fmt.Sprintf("%s://%s%s", parsedURL.Scheme, parsedURL.Host, parsedURL.Path)

	oauthParams := map[string]string{
		"oauth_consumer_key":     consumerKey,
		"oauth_nonce":            nonce,
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        timestamp,
		"oauth_token":            accessToken,
		"oauth_version":          "1.0",
	}

	keys := sortedKeys(oauthParams)
	paramPairs := make([]string, 0, len(keys))
	for _, k := range keys {
		paramPairs = append(paramPairs, fmt.Sprintf("%s=%s", percentEncodeRFC3986(k), percentEncodeRFC3986(oauthParams[k])))
	}
	paramString := strings.Join(paramPairs, "&")

	signatureBase := strings.Join([]string{
		strings.ToUpper(method),
		percentEncodeRFC3986(baseURL),
		percentEncodeRFC3986(paramString),
	}, "&")

	signingKey := fmt.Sprintf("%s&%s", percentEncodeRFC3986(consumerSecret), percentEncodeRFC3986(accessSecret))
	mac := hmac.New(sha1.New, []byte(signingKey))
	if _, writeErr := mac.Write([]byte(signatureBase)); writeErr != nil {
		return "", writeErr
	}
	oauthParams["oauth_signature"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))

	headerKeys := sortedKeys(oauthParams)
	headerParts := make([]string, 0, len(headerKeys))
	for _, k := range headerKeys {
		headerParts = append(headerParts, fmt.Sprintf(`%s="%s"`, percentEncodeRFC3986(k), percentEncodeRFC3986(oauthParams[k])))
	}
	return "OAuth " + strings.Join(headerParts, ", "), nil
}

func hasTwitterCredentials(cfg config.Config) bool {
	return cfg.TwitterClientKey != "" &&
		cfg.TwitterClientSecret != "" &&
		cfg.TwitterAccessToken != "" &&
		cfg.TwitterAccessTokenSecret != ""
}

func postTweetV2(client *http.Client, endpoint string, cfg config.Config, text string) error {
	if !hasTwitterCredentials(cfg) {
		return errors.New("twitter OAuth 1.0a credentials cannot be empty")
	}
	if endpoint == "" {
		endpoint = defaultTwitterV2Endpoint
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	bodyBytes, err := json.Marshal(tweetV2Request{Text: text})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	nonce, err := generateOAuthNonce()
	if err != nil {
		return err
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	authHeader, err := buildOAuth1Header(
		http.MethodPost,
		endpoint,
		cfg.TwitterClientKey,
		cfg.TwitterClientSecret,
		cfg.TwitterAccessToken,
		cfg.TwitterAccessTokenSecret,
		nonce,
		timestamp,
	)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	respBody, readErr := ioutil.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("twitter api v2 returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func runTwitterScheduler(svr server.Server, jobRepo *job.Repository, client *http.Client, endpoint string, now time.Time) {
	if !isWeekday(now) {
		log.Printf("skipping twitter job scheduler on weekend (%s)\n", now.UTC().Weekday())
		return
	}
	lastTwittedJobIDStr, err := jobRepo.GetValue(lastTwittedJobIDMetaKey)
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
	if jobsToPost <= 0 {
		jobsToPost = defaultTwitterJobsToPost
	}
	jobPosts, err := jobRepo.GetLastNJobsFromID(jobsToPost, lastTwittedJobID)
	if err != nil {
		svr.Log(err, "unable to retrieve jobs for twitter scheduler")
		return
	}
	log.Printf("found %d/%d jobs to post on twitter\n", len(jobPosts), jobsToPost)
	if len(jobPosts) == 0 {
		return
	}
	lastJobID := lastTwittedJobID
	postedCount := 0
	for _, j := range jobPosts {
		tweetText := formatTweetText(j, svr.GetConfig())
		if postErr := postTweetV2(client, endpoint, svr.GetConfig(), tweetText); postErr != nil {
			svr.Log(postErr, "unable to post tweet")
			continue
		}
		lastJobID = j.ID
		postedCount++
	}
	lastJobIDStr := strconv.Itoa(lastJobID)
	if setErr := jobRepo.SetValue(lastTwittedJobIDMetaKey, lastJobIDStr); setErr != nil {
		svr.Log(setErr, fmt.Sprintf("unable to save last twitter job id to db as %s", lastJobIDStr))
		return
	}
	log.Printf("updated last twitted job id to %s\n", lastJobIDStr)
	log.Printf("posted last %d jobs to twitter", postedCount)
}
