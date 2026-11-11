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

	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

// DefaultDailyTwitterJobs is the default number of jobs shared per weekday (issue #97).
const DefaultDailyTwitterJobs = 2

// TwitterV2TweetsEndpoint is the Twitter API v2 Manage Tweets endpoint.
var TwitterV2TweetsEndpoint = "https://api.twitter.com/2/tweets"

// IsTwitterPostingWeekday returns true only on Monday through Friday (weekdays only).
func IsTwitterPostingWeekday(t time.Time) bool {
	wd := t.UTC().Weekday()
	return wd != time.Saturday && wd != time.Sunday
}

// EffectiveDailyTwitterJobs returns the daily tweet limit (defaults to 2 jobs per weekday).
func EffectiveDailyTwitterJobs(configured int) int {
	if configured <= 0 || configured > DefaultDailyTwitterJobs {
		return DefaultDailyTwitterJobs
	}
	return configured
}

// FormatJobTweet formats a job post into the canonical tweet text.
func FormatJobTweet(j job.Post, siteJobCategory, urlProtocol, siteHost string) string {
	return fmt.Sprintf(
		"%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s%s/job/%s",
		j.JobTitle,
		j.Company,
		j.Location,
		j.SalaryRange,
		siteJobCategory,
		siteJobCategory,
		urlProtocol,
		siteHost,
		j.Slug,
	)
}

type twitterV2TweetRequest struct {
	Text string `json:"text"`
}

// BuildOAuth1Header constructs an OAuth 1.0a HMAC-SHA1 Authorization header for Twitter API v2 Manage Tweets.
func BuildOAuth1Header(method, rawURL, consumerKey, consumerSecret, accessToken, accessTokenSecret, nonce, timestamp string) (string, error) {
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

	keys := make([]string, 0, len(oauthParams))
	for k := range oauthParams {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	paramPairs := make([]string, 0, len(keys))
	for _, k := range keys {
		paramPairs = append(paramPairs, fmt.Sprintf("%s=%s", percentEncode(k), percentEncode(oauthParams[k])))
	}
	paramString := strings.Join(paramPairs, "&")

	signatureBase := strings.Join([]string{
		strings.ToUpper(method),
		percentEncode(baseURL),
		percentEncode(paramString),
	}, "&")

	signingKey := fmt.Sprintf("%s&%s", percentEncode(consumerSecret), percentEncode(accessTokenSecret))
	mac := hmac.New(sha1.New, []byte(signingKey))
	_, _ = mac.Write([]byte(signatureBase))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	oauthParams["oauth_signature"] = signature

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

// PostTweetV2 publishes a tweet using the Twitter API v2 Manage Tweets endpoint (POST /2/tweets).
func PostTweetV2(client *http.Client, endpoint, consumerKey, consumerSecret, accessToken, accessTokenSecret, text string) error {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if endpoint == "" {
		endpoint = TwitterV2TweetsEndpoint
	}
	payloadBytes, err := json.Marshal(twitterV2TweetRequest{Text: text})
	if err != nil {
		return err
	}

	nonceBytes := make([]byte, 16)
	_, _ = rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	authHeader, err := BuildOAuth1Header(
		http.MethodPost,
		endpoint,
		consumerKey,
		consumerSecret,
		accessToken,
		accessTokenSecret,
		nonce,
		timestamp,
	)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("twitter v2 api returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// TriggerTwitterJobsScheduler handles /x/task/twitter-jobs-scheduler and shares up to 2 jobs per day on weekdays only using Twitter API v2.
func TriggerTwitterJobsScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				now := time.Now().UTC()
				if !IsTwitterPostingWeekday(now) {
					log.Printf("skipping twitter-jobs-scheduler: %s is a weekend\n", now.Weekday().String())
					return
				}
				lastTwittedJobIDStr, err := jobRepo.GetValue("last_twitted_job_id")
				if err != nil {
					svr.Log(err, "unable to retrieve last twitter job id")
					return
				}
				lastTwittedJobID, err := strconv.Atoi(lastTwittedJobIDStr)
				if err != nil {
					svr.Log(err, "unable to convert job str to id")
					return
				}
				jobsToPost := EffectiveDailyTwitterJobs(svr.GetConfig().TwitterJobsToPost)
				jobPosts, err := jobRepo.GetLastNJobsFromID(jobsToPost, lastTwittedJobID)
				if err != nil {
					svr.Log(err, "unable to fetch jobs for twitter scheduler")
					return
				}
				log.Printf("found %d/%d weekday jobs to post on twitter\n", len(jobPosts), jobsToPost)
				if len(jobPosts) == 0 {
					return
				}
				lastJobID := lastTwittedJobID
				cfg := svr.GetConfig()
				httpClient := &http.Client{Timeout: 15 * time.Second}
				for _, j := range jobPosts {
					tweetText := FormatJobTweet(j, cfg.SiteJobCategory, cfg.URLProtocol, cfg.SiteHost)
					err := PostTweetV2(
						httpClient,
						TwitterV2TweetsEndpoint,
						cfg.TwitterClientKey,
						cfg.TwitterClientSecret,
						cfg.TwitterAccessToken,
						cfg.TwitterAccessTokenSecret,
						tweetText,
					)
					if err != nil {
						svr.Log(err, "unable to post tweet via twitter v2 api")
						continue
					}
					lastJobID = j.ID
				}
				lastJobIDStr := strconv.Itoa(lastJobID)
				err = jobRepo.SetValue("last_twitted_job_id", lastJobIDStr)
				if err != nil {
					svr.Log(err, fmt.Sprintf("unable to save last twitter job id to db as %s", lastJobIDStr))
					return
				}
				log.Printf("updated last twitted job id to %s\n", lastJobIDStr)
				log.Printf("posted last %d jobs to twitter via v2 API\n", len(jobPosts))
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}
