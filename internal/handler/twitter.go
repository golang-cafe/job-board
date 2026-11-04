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
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/server"
)

const (
	defaultTwitterV2Endpoint = "https://api.twitter.com/2/tweets"
	maxTweetLength           = 280
)

type tweetV2Request struct {
	Text string `json:"text"`
}

func isWeekday(now time.Time) bool {
	wd := now.UTC().Weekday()
	return wd >= time.Monday && wd <= time.Friday
}

func formatTweetText(j job.JobPost, cfg config.Config) string {
	head := fmt.Sprintf("%s with %s - %s | %s", j.JobTitle, j.Company, j.Location, j.SalaryRange)
	tail := fmt.Sprintf("\n\n#%s #%sjobs\n\n%s%s/job/%s", cfg.SiteJobCategory, cfg.SiteJobCategory, cfg.URLProtocol, cfg.SiteHost, j.Slug)
	full := head + tail
	if utf8.RuneCountInString(full) <= maxTweetLength {
		return full
	}
	tailRunes := utf8.RuneCountInString(tail)
	allowedHeadRunes := maxTweetLength - tailRunes - 1
	if allowedHeadRunes <= 0 {
		runes := []rune(full)
		return string(runes[:maxTweetLength])
	}
	headRunes := []rune(head)
	return string(headRunes[:allowedHeadRunes]) + "…" + tail
}

func oauthPercentEncode(s string) string {
	var buf bytes.Buffer
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_' || c == '~' {
			buf.WriteByte(c)
		} else {
			fmt.Fprintf(&buf, "%%%02X", c)
		}
	}
	return buf.String()
}

func generateOAuthNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(b)
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

	encodedPairs := make([]string, 0, len(keys))
	for _, k := range keys {
		encodedPairs = append(encodedPairs, oauthPercentEncode(k)+"="+oauthPercentEncode(params[k]))
	}
	paramString := strings.Join(encodedPairs, "&")

	baseURL := rawURL
	if idx := strings.IndexByte(rawURL, '?'); idx != -1 {
		baseURL = rawURL[:idx]
	}

	signatureBase := strings.ToUpper(method) + "&" + oauthPercentEncode(baseURL) + "&" + oauthPercentEncode(paramString)
	signingKey := oauthPercentEncode(consumerSecret) + "&" + oauthPercentEncode(accessTokenSecret)

	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(signatureBase))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	params["oauth_signature"] = signature

	headerKeys := make([]string, 0, len(params))
	for k := range params {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)

	headerParts := make([]string, 0, len(headerKeys))
	for _, k := range headerKeys {
		headerParts = append(headerParts, fmt.Sprintf(`%s="%s"`, oauthPercentEncode(k), oauthPercentEncode(params[k])))
	}
	return "OAuth " + strings.Join(headerParts, ", ")
}

func postTweetV2(httpClient *http.Client, endpoint string, cfg config.Config, text string) error {
	if endpoint == "" {
		endpoint = defaultTwitterV2Endpoint
	}
	payload, err := json.Marshal(tweetV2Request{Text: text})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	authHeader := buildOAuth1Header(
		http.MethodPost,
		endpoint,
		cfg.TwitterClientKey,
		cfg.TwitterClientSecret,
		cfg.TwitterAccessToken,
		cfg.TwitterAccessTokenSecret,
		generateOAuthNonce(),
		strconv.FormatInt(time.Now().UTC().Unix(), 10),
	)
	req.Header.Set("Authorization", authHeader)

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("twitter v2 api returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	return nil
}

func runTwitterScheduler(svr server.Server, jobRepo *job.Repository, httpClient *http.Client, endpoint string, now time.Time) {
	if !isWeekday(now) {
		log.Printf("skipping twitter scheduler on weekend (%s)\n", now.UTC().Weekday().String())
		return
	}

	lastTwittedJobIDStr, err := jobRepo.GetValue("last_twitted_job_id")
	if err != nil {
		svr.Log(err, "unable to retrieve last twitter job id")
		return
	}
	if strings.TrimSpace(lastTwittedJobIDStr) == "" {
		lastTwittedJobIDStr = "0"
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
		svr.Log(err, "unable to retrieve last n jobs from id for twitter scheduler")
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
		if err := postTweetV2(httpClient, endpoint, svr.GetConfig(), tweetText); err != nil {
			svr.Log(err, "unable to post tweet")
			continue
		}
		lastJobID = j.ID
		postedCount++
	}

	if lastJobID == lastTwittedJobID {
		return
	}

	lastJobIDStr := strconv.Itoa(lastJobID)
	if err := jobRepo.SetValue("last_twitted_job_id", lastJobIDStr); err != nil {
		svr.Log(err, fmt.Sprintf("unable to save last twitter job id to db as %s", lastJobIDStr))
		return
	}
	log.Printf("updated last twitted job id to %s\n", lastJobIDStr)
	log.Printf("posted last %d jobs to twitter", postedCount)
}
