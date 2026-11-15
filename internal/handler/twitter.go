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

const twitterV2ManageTweetsEndpoint = "https://api.twitter.com/2/tweets"

func TriggerTwitterJobsScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				now := time.Now().UTC()
				if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
					log.Printf("skipping twitter job scheduler on weekend (%s)\n", now.Weekday().String())
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
					jobsToPost = 2
				}

				jobPosts, err := jobRepo.GetLastNJobsFromID(jobsToPost, lastTwittedJobID)
				if err != nil {
					svr.Log(err, "unable to retrieve last n jobs for twitter scheduler")
					return
				}
				log.Printf("found %d/%d jobs to post on twitter\n", len(jobPosts), jobsToPost)
				if len(jobPosts) == 0 {
					return
				}

				lastJobID := lastTwittedJobID
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
					if err := postTweetV2(svr.GetConfig(), tweetText); err != nil {
						svr.Log(err, "unable to post tweet via twitter v2 api")
						continue
					}
					lastJobID = j.ID
				}

				lastJobIDStr := strconv.Itoa(lastJobID)
				if err := jobRepo.SetValue("last_twitted_job_id", lastJobIDStr); err != nil {
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

func postTweetV2(cfg config.Config, text string) error {
	payload, err := json.Marshal(map[string]string{
		"text": text,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, twitterV2ManageTweetsEndpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", buildTwitterOAuth1Header(
		http.MethodPost,
		twitterV2ManageTweetsEndpoint,
		cfg.TwitterClientKey,
		cfg.TwitterClientSecret,
		cfg.TwitterAccessToken,
		cfg.TwitterAccessTokenSecret,
	))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("twitter v2 api error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

func buildTwitterOAuth1Header(
	method, rawURL, consumerKey, consumerSecret, accessToken, accessSecret string,
) string {
	nonceBytes := make([]byte, 16)
	_, _ = rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

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

	signatureBase := fmt.Sprintf(
		"%s&%s&%s",
		strings.ToUpper(method),
		percentEncode(rawURL),
		percentEncode(paramString),
	)
	signingKey := fmt.Sprintf("%s&%s", percentEncode(consumerSecret), percentEncode(accessSecret))

	mac := hmac.New(sha1.New, []byte(signingKey))
	_, _ = mac.Write([]byte(signatureBase))
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
	return "OAuth " + strings.Join(headerParts, ", ")
}

func percentEncode(input string) string {
	return strings.ReplaceAll(url.QueryEscape(input), "+", "%20")
}
