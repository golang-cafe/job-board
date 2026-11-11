// X (Twitter) API v2 job sharing.
//
// This file replaces the retired Twitter API v1.1 client
// (github.com/ChimeraCoder/anaconda, used by the old TriggerTwitterScheduler)
// with a dependency-free X API v2 client that creates posts via
// POST https://api.twitter.com/2/tweets.
//
// Design notes (and how this differs from the earlier attempts at #97):
//
//   - Auth is OAuth 1.0a user context (HMAC-SHA1, RFC 5849) only. An
//     app-only bearer token CANNOT create posts on X API v2, so there is
//     deliberately no bearer-token fallback: missing/incomplete OAuth 1.0a
//     credentials fail fast before any HTTP request is made.
//   - The OAuth signature base string includes the request's query
//     parameters (RFC 5849 section 3.4.1.3.2). A signer that strips the
//     query string produces a wrong signature as soon as the endpoint URL
//     carries parameters; see TestBuildOAuth1HeaderRFC5849KnownAnswer.
//   - Posting is weekday-only in a configurable timezone
//     (TWITTER_TIMEZONE, default UTC), at most 2 jobs per day, and
//     once-per-day idempotent via the last_twitter_shared_date meta key.
//   - Failures are classified, not lumped together: a duplicate-content
//     403 or another permanent 4xx advances the cursor past the offending
//     job (otherwise one bad job would block every later job forever),
//     while auth failures (401/403), rate limiting (429, honouring
//     Retry-After) and 5xx/network errors abort the run and leave the
//     cursor at the last successfully posted job.
//   - The cursor is persisted after every posted/skipped job, not only at
//     the end of the run, so a crash mid-run does not re-post jobs.
//   - TWITTER_DRY_RUN=1 logs the exact tweet text instead of posting, so
//     the scheduler can be verified without spending paid X API quota.
//     (X charges per post; a post containing a URL is billed higher.)
//   - The scheduler core depends on the small twitterJobStore interface,
//     not on *sql.DB, so it is unit-testable with an in-memory fake
//     instead of a mock SQL driver tied to the exact SQL strings.
//
// No real X credentials are used anywhere in this repository; all secrets
// come from environment variables at runtime (see internal/config).
package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	_ "time/tzdata" // embed the IANA database so TWITTER_TIMEZONE works in slim containers

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

const (
	defaultTwitterV2Endpoint = "https://api.twitter.com/2/tweets"
	defaultTwitterTimezone   = "UTC"
	// X counts every URL as 23 characters (t.co) no matter how long it is,
	// and caps a post at 280 characters. See tweetWeightedLength.
	maxTweetWeightedLength   = 280
	twitterURLWeight         = 23
	maxDailyTweets           = 2
	lastTwittedJobIDKey      = "last_twitted_job_id"
	lastTwitterSharedDateKey = "last_twitter_shared_date"
)

var twitterSchedulerMu sync.Mutex

// twitterJobStore is the only persistence the scheduler needs. *job.Repository
// satisfies it, and tests substitute an in-memory fake.
type twitterJobStore interface {
	GetValue(key string) (string, error)
	SetValue(key, val string) error
	GetLastNJobsFromID(max, jobID int) ([]*job.JobPost, error)
}

// ---------------------------------------------------------------------------
// Tweet text
// ---------------------------------------------------------------------------

// tweetWeightedLength approximates how X counts a post: every URL counts as
// 23 characters, characters outside the Basic Multilingual Plane (most
// emoji) count as 2, everything else counts as 1. It is an approximation of
// the twitter-text rules, deliberately conservative in the safe direction
// only for astral characters; the URL rule is exact.
func tweetWeightedLength(s string) int {
	length := 0
	rest := s
	for {
		idx := strings.Index(rest, "http://")
		if idx == -1 {
			idx = strings.Index(rest, "https://")
		}
		if idx == -1 {
			break
		}
		length += utf8.RuneCountInString(rest[:idx])
		rest = rest[idx:]
		end := strings.IndexAny(rest, " \t\r\n")
		if end == -1 {
			length += twitterURLWeight
			return length
		}
		length += twitterURLWeight
		rest = rest[end:]
	}
	for _, r := range rest {
		if r >= 0x10000 {
			length += 2
		} else {
			length++
		}
	}
	return length
}

func formatTweetText(j job.JobPost, cfg config.Config) string {
	var headParts []string
	if strings.TrimSpace(j.JobTitle) != "" {
		headParts = append(headParts, strings.TrimSpace(j.JobTitle))
	}
	if strings.TrimSpace(j.Company) != "" {
		headParts = append(headParts, "with "+strings.TrimSpace(j.Company))
	}
	if strings.TrimSpace(j.Location) != "" {
		headParts = append(headParts, "- "+strings.TrimSpace(j.Location))
	}
	if strings.TrimSpace(j.SalaryRange) != "" {
		headParts = append(headParts, "| "+strings.TrimSpace(j.SalaryRange))
	}
	head := strings.Join(headParts, " ")
	link := fmt.Sprintf("%s%s/job/%s", cfg.URLProtocol, cfg.SiteHost, j.Slug)
	tail := fmt.Sprintf("\n\n#%s #%sjobs\n\n%s", cfg.SiteJobCategory, cfg.SiteJobCategory, link)

	full := head + tail
	if tweetWeightedLength(full) <= maxTweetWeightedLength {
		return full
	}
	// Truncate only the head, keep the tail (hashtags + job link) intact so
	// the post always links back to the job.
	tailWeight := tweetWeightedLength(tail)
	allowedHead := maxTweetWeightedLength - tailWeight - 1 // 1 for the "…" ellipsis
	if allowedHead <= 0 {
		runes := []rune(full)
		if len(runes) > maxTweetWeightedLength {
			runes = runes[:maxTweetWeightedLength]
		}
		return string(runes)
	}
	var b strings.Builder
	weight := 0
	for _, r := range head {
		w := 1
		if r >= 0x10000 {
			w = 2
		}
		if weight+w > allowedHead {
			break
		}
		b.WriteRune(r)
		weight += w
	}
	return strings.TrimRight(b.String(), " ") + "…" + tail
}

// ---------------------------------------------------------------------------
// OAuth 1.0a (RFC 5849) signing for X API v2 user-context requests
// ---------------------------------------------------------------------------

func oauthPercentEncode(s string) string {
	var buf bytes.Buffer
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
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

// signatureBaseString builds the RFC 5849 section 3.4.1 signature base
// string. Query parameters of the URL are part of the signature input, and
// the JSON request body is not (only oauth_* and query parameters are).
func signatureBaseString(method, rawURL string, oauthParams map[string]string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		u = &url.URL{Path: rawURL}
	}
	type kv struct{ k, v string }
	pairs := make([]kv, 0, len(oauthParams)+4)
	for k, v := range oauthParams {
		if k == "oauth_signature" {
			continue
		}
		pairs = append(pairs, kv{k, v})
	}
	if u != nil {
		for k, vs := range u.Query() {
			for _, v := range vs {
				pairs = append(pairs, kv{k, v})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k == pairs[j].k {
			return pairs[i].v < pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	encoded := make([]string, 0, len(pairs))
	for _, p := range pairs {
		encoded = append(encoded, oauthPercentEncode(p.k)+"="+oauthPercentEncode(p.v))
	}
	baseURL := rawURL
	if u != nil {
		u.RawQuery = ""
		u.Fragment = ""
		baseURL = u.String()
	}
	return strings.ToUpper(method) + "&" + oauthPercentEncode(baseURL) + "&" +
		oauthPercentEncode(strings.Join(encoded, "&"))
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
	base := signatureBaseString(method, rawURL, params)
	signingKey := oauthPercentEncode(consumerSecret) + "&" + oauthPercentEncode(accessTokenSecret)
	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(base))
	params["oauth_signature"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, oauthPercentEncode(k), oauthPercentEncode(params[k])))
	}
	return "OAuth " + strings.Join(parts, ", ")
}

// twitterOAuthCredentialsOK reports whether the four OAuth 1.0a user-context
// values are all present. A bearer token alone is NOT sufficient to create
// posts on X API v2, so it is never treated as a substitute here.
func twitterOAuthCredentialsOK(cfg config.Config) bool {
	return cfg.TwitterClientKey != "" && cfg.TwitterClientSecret != "" &&
		cfg.TwitterAccessToken != "" && cfg.TwitterAccessTokenSecret != ""
}

// ---------------------------------------------------------------------------
// POST /2/tweets
// ---------------------------------------------------------------------------

type tweetV2Request struct {
	Text string `json:"text"`
}

type tweetV2Response struct {
	Data *struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"data"`
}

// xPostError classifies a failed post attempt so the scheduler can decide
// whether to skip the job, abort the run, or back off.
type xPostError struct {
	StatusCode int    // 0 for a transport error (no HTTP response)
	Title      string // X error title, when the API returned one
	Detail     string // X error detail, when the API returned one
	RetryAfter time.Duration
	Duplicate  bool // 403: this exact content was already posted
	Auth       bool // 401, or 403 that is not a duplicate (bad/revoked credentials, no permission)
	Permanent  bool // other 4xx: retrying the same job will never succeed
	Transient  bool // 429, 5xx, or transport error: retry later
}

func (e *xPostError) Error() string {
	if e.StatusCode == 0 {
		return fmt.Sprintf("twitter v2 api request failed: %s", e.Detail)
	}
	if e.Detail != "" {
		return fmt.Sprintf("twitter v2 api returned status %d: %s", e.StatusCode, e.Detail)
	}
	return fmt.Sprintf("twitter v2 api returned status %d", e.StatusCode)
}

// classifyPostError turns an X API v2 error response into an xPostError.
// body is the (already size-limited) raw response body.
func classifyPostError(statusCode int, body []byte, retryAfterHeader string) *xPostError {
	err := &xPostError{StatusCode: statusCode}
	var parsed struct {
		Title  string `json:"title"`
		Detail string `json:"detail"`
		Errors []struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
			Type   string `json:"type"`
		} `json:"errors"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &parsed)
	}
	if parsed.Title != "" {
		err.Title = parsed.Title
	}
	if parsed.Detail != "" {
		err.Detail = parsed.Detail
	}
	for _, e := range parsed.Errors {
		if err.Title == "" {
			err.Title = e.Title
		}
		if err.Detail == "" {
			err.Detail = e.Detail
		}
		if strings.Contains(strings.ToLower(e.Detail), "duplicate") {
			err.Duplicate = true
		}
	}
	if err.Detail == "" {
		err.Detail = strings.TrimSpace(string(body))
	}
	if secs, convErr := strconv.Atoi(strings.TrimSpace(retryAfterHeader)); convErr == nil && secs > 0 {
		err.RetryAfter = time.Duration(secs) * time.Second
	}
	switch {
	case statusCode == 429:
		err.Transient = true
	case statusCode == http.StatusUnauthorized:
		err.Auth = true
	case statusCode == http.StatusForbidden && err.Duplicate:
		err.Permanent = true
	case statusCode == http.StatusForbidden:
		err.Auth = true
	case statusCode >= 400 && statusCode < 500:
		err.Permanent = true
	case statusCode >= 500:
		err.Transient = true
	}
	return err
}

// postTweetV2 creates one post via X API v2. It never touches real
// credentials in tests: endpoint is injectable (TWITTER_API_URL) and the
// caller supplies the HTTP client, so tests point it at httptest servers.
func postTweetV2(ctx context.Context, httpClient *http.Client, endpoint string, cfg config.Config, text string, now time.Time) (string, error) {
	if !twitterOAuthCredentialsOK(cfg) {
		return "", errors.New("twitter OAuth 1.0a credentials are not configured " +
			"(TWITTER_CLIENT_KEY, TWITTER_CLIENT_SECRET, TWITTER_ACCESS_TOKEN and " +
			"TWITTER_ACCESS_TOKEN_SECRET must all be set; an app-only bearer token " +
			"cannot create posts on X API v2)")
	}
	if endpoint == "" {
		endpoint = cfg.TwitterAPIURL
	}
	if endpoint == "" {
		endpoint = defaultTwitterV2Endpoint
	}
	payload, err := json.Marshal(tweetV2Request{Text: text})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", buildOAuth1Header(
		http.MethodPost, endpoint,
		cfg.TwitterClientKey, cfg.TwitterClientSecret,
		cfg.TwitterAccessToken, cfg.TwitterAccessTokenSecret,
		generateOAuthNonce(), strconv.FormatInt(now.UTC().Unix(), 10),
	))

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", &xPostError{Detail: err.Error(), Transient: true}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", classifyPostError(resp.StatusCode, body, resp.Header.Get("Retry-After"))
	}
	var created tweetV2Response
	_ = json.Unmarshal(body, &created)
	if created.Data != nil {
		return created.Data.ID, nil
	}
	return "", nil
}

// ---------------------------------------------------------------------------
// Scheduler
// ---------------------------------------------------------------------------

func resolveDailyTweetLimit(configured int) int {
	if configured <= 0 || configured > maxDailyTweets {
		return maxDailyTweets
	}
	return configured
}

func loadTwitterLocation(cfg config.Config) (*time.Location, error) {
	name := strings.TrimSpace(cfg.TwitterTimezone)
	if name == "" {
		name = defaultTwitterTimezone
	}
	return time.LoadLocation(name)
}

func isWeekday(t time.Time) bool {
	wd := t.Weekday()
	return wd >= time.Monday && wd <= time.Friday
}

// twitterSchedulerResult summarises one scheduler run for logging and tests.
type twitterSchedulerResult struct {
	Skipped     string // "", "weekend", "already-shared-today", "no-jobs", "dry-run", "already-running", "not-configured"
	Posted      int
	SkippedJobs int
	Failed      int
	RetryAfter  time.Duration
	Err         error
}

// runTwitterScheduler posts up to resolveDailyTweetLimit new jobs for one
// weekday. It is safe to trigger repeatedly (e.g. from cron): weekends,
// same-day repeats and concurrent triggers are all no-ops.
func runTwitterScheduler(svr server.Server, store twitterJobStore, httpClient *http.Client, endpoint string, now time.Time) twitterSchedulerResult {
	if !twitterSchedulerMu.TryLock() {
		log.Println("twitter scheduler is already running; skipping concurrent trigger")
		return twitterSchedulerResult{Skipped: "already-running"}
	}
	defer twitterSchedulerMu.Unlock()

	cfg := svr.GetConfig()
	loc, err := loadTwitterLocation(cfg)
	if err != nil {
		svr.Log(err, "invalid TWITTER_TIMEZONE; refusing to guess which day it is")
		return twitterSchedulerResult{Err: err}
	}
	localNow := now.In(loc)
	if !isWeekday(localNow) {
		log.Printf("skipping twitter scheduler on weekend (%s, %s)\n", localNow.Weekday(), loc)
		return twitterSchedulerResult{Skipped: "weekend"}
	}
	if !twitterOAuthCredentialsOK(cfg) && !cfg.TwitterDryRun {
		svr.Log(errors.New("twitter OAuth 1.0a credentials are not configured"), "unable to post jobs to twitter")
		return twitterSchedulerResult{Skipped: "not-configured", Err: errors.New("twitter OAuth 1.0a credentials are not configured")}
	}

	todayStr := localNow.Format("2006-01-02")
	if lastShared, err := store.GetValue(lastTwitterSharedDateKey); err == nil && strings.TrimSpace(lastShared) == todayStr {
		log.Printf("skipping twitter scheduler: already shared jobs today (%s)\n", todayStr)
		return twitterSchedulerResult{Skipped: "already-shared-today"}
	}

	lastJobID, err := lastTwitterJobID(store)
	if err != nil {
		svr.Log(err, "unable to retrieve last twitter job id")
		return twitterSchedulerResult{Err: err}
	}

	jobsToPost := resolveDailyTweetLimit(cfg.TwitterJobsToPost)
	jobPosts, err := store.GetLastNJobsFromID(jobsToPost, lastJobID)
	if err != nil {
		svr.Log(err, "unable to retrieve jobs for twitter scheduler")
		return twitterSchedulerResult{Err: err}
	}
	log.Printf("found %d/%d jobs to post on twitter\n", len(jobPosts), jobsToPost)
	if len(jobPosts) == 0 {
		return twitterSchedulerResult{Skipped: "no-jobs"}
	}

	if cfg.TwitterDryRun {
		for _, j := range jobPosts {
			if j == nil {
				continue
			}
			log.Printf("[twitter dry-run] would post job %d: %q\n", j.ID, formatTweetText(*j, cfg))
		}
		return twitterSchedulerResult{Skipped: "dry-run"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result := twitterSchedulerResult{}
	cursorAdvanced := false
	for _, j := range jobPosts {
		if j == nil {
			continue
		}
		tweetID, err := postTweetV2(ctx, httpClient, endpoint, cfg, formatTweetText(*j, cfg), localNow)
		if err == nil {
			log.Printf("posted job %d to twitter (tweet id %s)\n", j.ID, tweetID)
			result.Posted++
			if err := advanceTwitterCursor(store, j.ID); err != nil {
				svr.Log(err, "unable to save last twitter job id; aborting run to avoid re-posting")
				result.Err = err
				result.Failed++
				return result
			}
			lastJobID = j.ID
			cursorAdvanced = true
			continue
		}
		var postErr *xPostError
		if errors.As(err, &postErr) && (postErr.Duplicate || postErr.Permanent) {
			// This job can never be posted (already posted / invalid). Skip
			// it permanently instead of letting it block every later job.
			svr.Log(err, fmt.Sprintf("skipping job %d permanently (status %d)", j.ID, postErr.StatusCode))
			result.SkippedJobs++
			if err := advanceTwitterCursor(store, j.ID); err != nil {
				svr.Log(err, "unable to save last twitter job id; aborting run")
				result.Err = err
				result.Failed++
				return result
			}
			lastJobID = j.ID
			cursorAdvanced = true
			continue
		}
		// Auth failure, rate limit, 5xx or network error: stop the run and
		// leave the cursor at the last posted job so the next run retries
		// this job instead of skipping it.
		svr.Log(err, fmt.Sprintf("unable to post job %d to twitter; aborting run", j.ID))
		result.Failed++
		if postErr != nil {
			result.RetryAfter = postErr.RetryAfter
		}
		result.Err = err
		break
	}

	// Mark the day as done once the cursor moved: without this, a second
	// trigger on the same day would post the next jobs and break the
	// 2-jobs-per-day cap.
	if cursorAdvanced {
		if err := store.SetValue(lastTwitterSharedDateKey, todayStr); err != nil {
			svr.Log(err, fmt.Sprintf("unable to save last twitter shared date %s (cursor already advanced; a repeat trigger today may post extra jobs)", todayStr))
		}
	}
	log.Printf("twitter scheduler finished: posted=%d skipped=%d failed=%d\n", result.Posted, result.SkippedJobs, result.Failed)
	return result
}

// lastTwitterJobID reads the posting cursor. A missing meta row (fresh
// database, sql.ErrNoRows) or a blank value means "start from the beginning"
// and is handled here, without changing GetValue's contract for other
// callers (Telegram etc. still see the error).
func lastTwitterJobID(store twitterJobStore) (int, error) {
	raw, err := store.GetValue(lastTwittedJobIDKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("unable to convert last twitter job id %q to int: %w", raw, err)
	}
	return id, nil
}

func advanceTwitterCursor(store twitterJobStore, jobID int) error {
	return store.SetValue(lastTwittedJobIDKey, strconv.Itoa(jobID))
}

// TriggerTwitterJobsScheduler is the machine-authenticated cron endpoint for
// the X API v2 scheduler: POST /x/task/twitter-jobs-scheduler with the
// x-machine-token header. It answers immediately (like the other /x/task
// handlers) and posts in the background.
func TriggerTwitterJobsScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				runTwitterScheduler(
					svr,
					jobRepo,
					&http.Client{Timeout: 15 * time.Second},
					svr.GetConfig().TwitterAPIURL,
					time.Now(),
				)
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}
