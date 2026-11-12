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

const twitterV2ManageTweetsURL = "https://api.twitter.com/2/tweets"

func isWeekday(t time.Time) bool {
	wd := t.UTC().Weekday()
	return wd != time.Saturday && wd != time.Sunday
}

func formatJobTweet(j job.JobPost, category, protocol, host string) string {
	return fmt.Sprintf(
		"%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s%s/job/%s",
		j.JobTitle,
		j.Company,
		j.Location,
		j.SalaryRange,
		category,
		category,
		protocol,
		host,
		j.Slug,
	)
}

// TriggerTwitterJobsScheduler shares daily jobs (weekdays only, default 2 jobs/day) using Twitter API v2 (/2/tweets).
func TriggerTwitterJobsScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			if !isWeekday(time.Now().UTC()) {
				svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "skipped_weekend"})
				return
			}
			go func() {
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
				jobsToPost := svr.GetConfig().TwitterJobsToPost
				if jobsToPost <= 0 {
					jobsToPost = 2
				}
				jobPosts, err := jobRepo.GetLastNJobsFromID(jobsToPost, lastTwittedJobID)
				log.Printf("found %d/%d jobs to post on twitter\n", len(jobPosts), jobsToPost)
				if len(jobPosts) == 0 {
					return
				}
				lastJobID := lastTwittedJobID
				cfg := svr.GetConfig()
				for _, j := range jobPosts {
					tweetText := formatJobTweet(j, cfg.SiteJobCategory, cfg.URLProtocol, cfg.SiteHost)
					err := postTweetV2(
						http.DefaultClient,
						twitterV2ManageTweetsURL,
						tweetText,
						cfg.TwitterClientKey,
						cfg.TwitterClientSecret,
						cfg.TwitterAccessToken,
						cfg.TwitterAccessTokenSecret,
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
				log.Printf("posted last %d jobs to twitter", len(jobPosts))
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func postTweetV2(client *http.Client, endpoint, text, consumerKey, consumerSecret, accessToken, accessSecret string) error {
	payload, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	authHeader := buildOAuth1Header(http.MethodPost, endpoint, consumerKey, consumerSecret, accessToken, accessSecret, time.Now().UTC())
	req.Header.Set("Authorization", authHeader)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("twitter v2 api returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func buildOAuth1Header(method, rawURL, consumerKey, consumerSecret, accessToken, accessSecret string, now time.Time) string {
	nonceBytes := make([]byte, 16)
	_, _ = rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)
	timestamp := strconv.FormatInt(now.Unix(), 10)

	params := map[string]string{
		"oauth_consumer_key":     consumerKey,
		"oauth_nonce":            nonce,
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        timestamp,
		"oauth_token":            accessToken,
		"oauth_version":          "1.0",
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var paramPairs []string
	for _, k := range keys {
		paramPairs = append(paramPairs, fmt.Sprintf("%s=%s", percentEncode(k), percentEncode(params[k])))
	}
	paramString := strings.Join(paramPairs, "&")
	baseString := strings.ToUpper(method) + "&" + percentEncode(rawURL) + "&" + percentEncode(paramString)
	signingKey := percentEncode(consumerSecret) + "&" + percentEncode(accessSecret)

	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(baseString))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	params["oauth_signature"] = signature
	keys = append(keys, "oauth_signature")
	sort.Strings(keys)

	var headerParts []string
	for _, k := range keys {
		headerParts = append(headerParts, fmt.Sprintf(`%s="%s"`, percentEncode(k), percentEncode(params[k])))
	}
	return "OAuth " + strings.Join(headerParts, ", ")
}

func percentEncode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}
