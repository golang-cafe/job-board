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

var twitterV2TweetsEndpoint = "https://api.twitter.com/2/tweets"

const defaultWeekdayTwitterJobsToPost = 2

type twitterV2CreateTweetRequest struct {
	Text string `json:"text"`
}

// IsWeekday returns true when t falls on Monday through Friday.
func IsWeekday(t time.Time) bool {
	wd := t.UTC().Weekday()
	return wd >= time.Monday && wd <= time.Friday
}

func oauthPercentEncode(s string) string {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func buildOAuth1Header(method, rawURL, consumerKey, consumerSecret, accessToken, accessTokenSecret, nonce, timestamp string) string {
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
		paramPairs = append(paramPairs, fmt.Sprintf("%s=%s", oauthPercentEncode(k), oauthPercentEncode(params[k])))
	}
	parameterString := strings.Join(paramPairs, "&")

	baseString := strings.ToUpper(method) + "&" + oauthPercentEncode(rawURL) + "&" + oauthPercentEncode(parameterString)
	signingKey := oauthPercentEncode(consumerSecret) + "&" + oauthPercentEncode(accessTokenSecret)

	mac := hmac.New(sha1.New, []byte(signingKey))
	_, _ = mac.Write([]byte(baseString))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	params["oauth_signature"] = signature

	keys = append(keys, "oauth_signature")
	sort.Strings(keys)

	var headerParts []string
	for _, k := range keys {
		headerParts = append(headerParts, fmt.Sprintf(`%s="%s"`, oauthPercentEncode(k), oauthPercentEncode(params[k])))
	}
	return "OAuth " + strings.Join(headerParts, ", ")
}

func generateOAuthNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b)
}

func PostTweetV2(client *http.Client, cfg config.Config, text string) error {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	payload := twitterV2CreateTweetRequest{Text: text}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("unable to marshal tweet payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, twitterV2TweetsEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("unable to create twitter v2 request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if cfg.TwitterClientKey != "" && cfg.TwitterClientSecret != "" && cfg.TwitterAccessTokenSecret != "" {
		authHeader := buildOAuth1Header(
			http.MethodPost,
			twitterV2TweetsEndpoint,
			cfg.TwitterClientKey,
			cfg.TwitterClientSecret,
			cfg.TwitterAccessToken,
			cfg.TwitterAccessTokenSecret,
			generateOAuthNonce(),
			strconv.FormatInt(time.Now().UTC().Unix(), 10),
		)
		req.Header.Set("Authorization", authHeader)
	} else {
		req.Header.Set("Authorization", "Bearer "+cfg.TwitterAccessToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("unable to send twitter v2 request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("twitter v2 API returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func TriggerTwitterJobsScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				if !IsWeekday(time.Now().UTC()) {
					log.Println("skipping twitter jobs scheduler: today is not a weekday")
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
				jobsToPost := svr.GetConfig().TwitterJobsToPost
				if jobsToPost <= 0 {
					jobsToPost = defaultWeekdayTwitterJobsToPost
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
				httpClient := &http.Client{Timeout: 15 * time.Second}
				for _, j := range jobPosts {
					tweetText := fmt.Sprintf(
						"%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s%s/job/%s",
						j.JobTitle,
						j.Company,
						j.Location,
						j.SalaryRange,
						svr.GetConfig().SiteJobCategory,
						svr.GetConfig().SiteJobCategory,
						svr.GetConfig().URLProtocol,
						svr.GetConfig().SiteHost,
						j.Slug,
					)
					if err := PostTweetV2(httpClient, svr.GetConfig(), tweetText); err != nil {
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
