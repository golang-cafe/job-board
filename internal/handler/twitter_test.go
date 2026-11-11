package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/email"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/server"
)

// ---------------------------------------------------------------------------
// In-memory fake for twitterJobStore. The scheduler only depends on this
// small interface, so tests do not need a database or a mock SQL driver
// tied to the exact SQL strings in the repository.
// ---------------------------------------------------------------------------

type fakeTwitterStore struct {
	mu   sync.Mutex
	meta map[string]string
	jobs []*job.JobPost
}

func newFakeTwitterStore(jobs []*job.JobPost, meta map[string]string) *fakeTwitterStore {
	if meta == nil {
		meta = map[string]string{}
	}
	return &fakeTwitterStore{meta: meta, jobs: jobs}
}

func (f *fakeTwitterStore) GetValue(key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.meta[key]
	if !ok {
		return "", sql.ErrNoRows
	}
	return v, nil
}

func (f *fakeTwitterStore) SetValue(key, val string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.meta[key] = val
	return nil
}

func (f *fakeTwitterStore) GetLastNJobsFromID(max, jobID int) ([]*job.JobPost, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*job.JobPost
	for _, j := range f.jobs {
		if j != nil && j.ID > jobID {
			out = append(out, j)
			if len(out) >= max {
				break
			}
		}
	}
	return out, nil
}

func (f *fakeTwitterStore) getMeta(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.meta[key]
}

// ---------------------------------------------------------------------------
// Fake X API server
// ---------------------------------------------------------------------------

type fakeXBehavior struct {
	status     int
	body       string
	retryAfter string
}

type fakeXServer struct {
	t         *testing.T
	server    *httptest.Server
	mu        sync.Mutex
	calls     []string // tweet texts, in order
	authHeads []string
	behaviors map[string]fakeXBehavior // keyed by job slug found in the text
	def       fakeXBehavior
}

func newFakeXServer(t *testing.T, behaviors map[string]fakeXBehavior, def fakeXBehavior) *fakeXServer {
	t.Helper()
	fx := &fakeXServer{t: t, behaviors: behaviors, def: def}
	fx.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		var payload tweetV2Request
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode tweet payload: %v", err)
		}
		fx.mu.Lock()
		fx.calls = append(fx.calls, payload.Text)
		fx.authHeads = append(fx.authHeads, r.Header.Get("Authorization"))
		fx.mu.Unlock()
		b := def
		for slug, beh := range behaviors {
			if strings.Contains(payload.Text, slug) {
				b = beh
				break
			}
		}
		if b.retryAfter != "" {
			w.Header().Set("Retry-After", b.retryAfter)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(b.status)
		if b.body != "" {
			_, _ = w.Write([]byte(b.body))
		} else {
			_, _ = w.Write([]byte(`{"data":{"id":"123","text":"ok"}}`))
		}
	}))
	t.Cleanup(fx.server.Close)
	return fx
}

func (fx *fakeXServer) callCount() int {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	return len(fx.calls)
}

func testTwitterConfig() config.Config {
	return config.Config{
		TwitterJobsToPost:        5, // capped to 2 per day by the scheduler
		TwitterClientKey:         "key",
		TwitterClientSecret:      "secret",
		TwitterAccessToken:       "token",
		TwitterAccessTokenSecret: "tokensecret",
		TwitterAPIURL:            "",
		TwitterTimezone:          "UTC",
		SiteJobCategory:          "golang",
		URLProtocol:              "https://",
		SiteHost:                 "golang.cafe",
	}
}

func testServer(cfg config.Config) server.Server {
	return server.NewServer(cfg, nil, nil, nil, email.Client{}, nil)
}

func testJobs() []*job.JobPost {
	return []*job.JobPost{
		{ID: 11, JobTitle: "Go Backend Engineer", Company: "Beta", SalaryRange: "$140k", Location: "Remote", Slug: "go-backend-beta"},
		{ID: 12, JobTitle: "Go Platform Engineer", Company: "Gamma", SalaryRange: "$160k", Location: "Berlin", Slug: "go-platform-gamma"},
		{ID: 13, JobTitle: "Go Runtime Engineer", Company: "Delta", SalaryRange: "$175k", Location: "London", Slug: "go-runtime-delta"},
	}
}

// 2026-11-04 is a Wednesday, 2026-11-07 a Saturday, 2026-11-08 a Sunday.
func wednesday() time.Time { return time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC) }
func thursday() time.Time  { return time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC) }
func saturday() time.Time  { return time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC) }

// ---------------------------------------------------------------------------
// OAuth 1.0a signing
// ---------------------------------------------------------------------------

// TestBuildOAuth1HeaderRFC5849KnownAnswer checks the signer against the
// worked example in RFC 5849 section 3.4. A signer that merely emits the
// right header fields (or that drops the URL query string from the base
// string) fails this test.
func TestBuildOAuth1HeaderRFC5849KnownAnswer(t *testing.T) {
	params := map[string]string{
		"oauth_consumer_key":     "dpf43f3p2l4k3l03",
		"oauth_nonce":            "kllo9940pd9333jh",
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        "1191242096",
		"oauth_token":            "nnch734d00sl2jdk",
		"oauth_version":          "1.0",
	}
	wantBase := "GET&http%3A%2F%2Fphotos.example.net%2Fphotos&" +
		"file%3Dvacation.jpg%26oauth_consumer_key%3Ddpf43f3p2l4k3l03%26" +
		"oauth_nonce%3Dkllo9940pd9333jh%26oauth_signature_method%3DHMAC-SHA1%26" +
		"oauth_timestamp%3D1191242096%26oauth_token%3Dnnch734d00sl2jdk%26" +
		"oauth_version%3D1.0%26size%3Doriginal"
	gotBase := signatureBaseString("GET", "http://photos.example.net/photos?file=vacation.jpg&size=original", params)
	if gotBase != wantBase {
		t.Fatalf("signature base string mismatch:\n got %q\nwant %q", gotBase, wantBase)
	}
	hdr := buildOAuth1Header("GET", "http://photos.example.net/photos?file=vacation.jpg&size=original",
		"dpf43f3p2l4k3l03", "kd94hf93k423kf44", "nnch734d00sl2jdk", "pfkkdhi9sl3r4s00",
		"kllo9940pd9333jh", "1191242096")
	// RFC 5849 section 3.4.2: signature "tR3+Ty81lMeYAr/Fid0kMTYa/WM=",
	// percent-encoded in the Authorization header.
	if !strings.Contains(hdr, `oauth_signature="tR3%2BTy81lMeYAr%2FFid0kMTYa%2FWM%3D"`) {
		t.Fatalf("expected RFC 5849 signature in header, got %q", hdr)
	}
}

func TestBuildOAuth1HeaderQueryChangesSignature(t *testing.T) {
	h1 := buildOAuth1Header("POST", "https://api.twitter.com/2/tweets?foo=bar", "k", "s", "t", "ts", "n", "1700000000")
	h2 := buildOAuth1Header("POST", "https://api.twitter.com/2/tweets?foo=baz", "k", "s", "t", "ts", "n", "1700000000")
	if h1 == h2 {
		t.Fatalf("expected different signatures for different query strings, got identical headers")
	}
}

// ---------------------------------------------------------------------------
// Tweet text
// ---------------------------------------------------------------------------

func TestFormatTweetTextWeightedLengthAndLink(t *testing.T) {
	cfg := testTwitterConfig()
	j := job.JobPost{
		JobTitle:    strings.Repeat("Senior Go Distributed Systems Architect 🚀 ", 8),
		Company:     "Acme Cloud Corp",
		Location:    "Remote (Worldwide)",
		SalaryRange: "$180k - $240k",
		Slug:        "senior-go-architect-acme",
	}
	txt := formatTweetText(j, cfg)
	if !utf8.ValidString(txt) {
		t.Fatalf("expected valid UTF-8 tweet text")
	}
	if tweetWeightedLength(txt) > maxTweetWeightedLength {
		t.Fatalf("expected weighted length <= %d, got %d: %q", maxTweetWeightedLength, tweetWeightedLength(txt), txt)
	}
	if !strings.HasSuffix(txt, "https://golang.cafe/job/senior-go-architect-acme") {
		t.Fatalf("expected canonical job link preserved at end of tweet, got %q", txt)
	}
}

func TestFormatTweetTextURLCountsAs23(t *testing.T) {
	cfg := testTwitterConfig()
	// A 250-char slug makes the raw text far longer than 280 runes, but X
	// counts the URL as 23 chars, so the tweet must NOT be truncated.
	j := job.JobPost{
		JobTitle: "Go Engineer",
		Company:  "Beta",
		Location: "Remote",
		Slug:     strings.Repeat("a", 250),
	}
	txt := formatTweetText(j, cfg)
	if !strings.Contains(txt, strings.Repeat("a", 250)) {
		t.Fatalf("expected full slug preserved (URL counts as 23 chars), got %q", txt)
	}
	if tweetWeightedLength(txt) > maxTweetWeightedLength {
		t.Fatalf("expected weighted length <= %d, got %d", maxTweetWeightedLength, tweetWeightedLength(txt))
	}
}

func TestFormatTweetTextSkipsEmptyFields(t *testing.T) {
	cfg := testTwitterConfig()
	txt := formatTweetText(job.JobPost{JobTitle: "Go Engineer", Slug: "go-engineer"}, cfg)
	if strings.Contains(txt, "with") || strings.Contains(txt, " - ") || strings.Contains(txt, " | ") {
		t.Fatalf("expected no empty-field placeholders, got %q", txt)
	}
	if !strings.HasPrefix(txt, "Go Engineer") {
		t.Fatalf("expected tweet to start with the title, got %q", txt)
	}
}

func TestResolveDailyTweetLimit(t *testing.T) {
	for in, want := range map[int]int{0: 2, -1: 2, 1: 1, 2: 2, 5: 2, 100: 2} {
		if got := resolveDailyTweetLimit(in); got != want {
			t.Fatalf("resolveDailyTweetLimit(%d) = %d, want %d", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Error classification
// ---------------------------------------------------------------------------

func TestClassifyPostError(t *testing.T) {
	dupBody := []byte(`{"errors":[{"detail":"You are not allowed to create a Tweet with duplicate content.","title":"Forbidden","type":"https://api.twitter.com/2/problems/duplicate-content"}]}`)
	cases := []struct {
		name       string
		status     int
		body       []byte
		retryAfter string
		check      func(*xPostError) bool
	}{
		{"rate limited", 429, []byte(`{"title":"Too Many Requests"}`), "60",
			func(e *xPostError) bool { return e.Transient && e.RetryAfter == 60*time.Second }},
		{"duplicate", 403, dupBody, "",
			func(e *xPostError) bool { return e.Duplicate && e.Permanent && !e.Auth }},
		{"forbidden other", 403, []byte(`{"detail":"You are not permitted to perform this action."}`), "",
			func(e *xPostError) bool { return e.Auth && !e.Duplicate }},
		{"unauthorized", 401, []byte(`{"title":"Unauthorized"}`), "",
			func(e *xPostError) bool { return e.Auth }},
		{"bad request", 400, []byte(`{"detail":"Invalid request."}`), "",
			func(e *xPostError) bool { return e.Permanent && !e.Transient }},
		{"server error", 500, []byte(`{"title":"Internal Server Error"}`), "",
			func(e *xPostError) bool { return e.Transient }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyPostError(tc.status, tc.body, tc.retryAfter)
			if !tc.check(got) {
				t.Fatalf("classifyPostError(%d) = %+v, failed check", tc.status, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// postTweetV2
// ---------------------------------------------------------------------------

func TestPostTweetV2RequiresOAuthCredentials(t *testing.T) {
	fx := newFakeXServer(t, nil, fakeXBehavior{status: http.StatusCreated})
	cfg := testTwitterConfig()
	cfg.TwitterClientKey = ""
	cfg.TwitterClientSecret = ""
	cfg.TwitterAccessToken = ""
	cfg.TwitterAccessTokenSecret = ""
	_, err := postTweetV2(context.Background(), fx.server.Client(), fx.server.URL, cfg, "hello", time.Now())
	if err == nil {
		t.Fatalf("expected error with no OAuth credentials")
	}
	if !strings.Contains(err.Error(), "OAuth 1.0a") || !strings.Contains(err.Error(), "bearer") {
		t.Fatalf("expected error to explain OAuth 1.0a is required and bearer is insufficient, got %q", err)
	}
	if fx.callCount() != 0 {
		t.Fatalf("expected no HTTP request without credentials, got %d", fx.callCount())
	}
}

func TestPostTweetV2SendsOAuthHeader(t *testing.T) {
	fx := newFakeXServer(t, nil, fakeXBehavior{status: http.StatusCreated})
	_, err := postTweetV2(context.Background(), fx.server.Client(), fx.server.URL, testTwitterConfig(), "hello", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fx.callCount() != 1 {
		t.Fatalf("expected 1 request, got %d", fx.callCount())
	}
	fx.mu.Lock()
	auth := fx.authHeads[0]
	fx.mu.Unlock()
	if !strings.HasPrefix(auth, "OAuth ") || !strings.Contains(auth, `oauth_signature_method="HMAC-SHA1"`) {
		t.Fatalf("expected OAuth 1.0a Authorization header, got %q", auth)
	}
}

// ---------------------------------------------------------------------------
// Scheduler
// ---------------------------------------------------------------------------

func TestRunTwitterSchedulerWeekdayCapAndIdempotency(t *testing.T) {
	fx := newFakeXServer(t, nil, fakeXBehavior{status: http.StatusCreated})
	cfg := testTwitterConfig()
	svr := testServer(cfg)
	store := newFakeTwitterStore(testJobs(), map[string]string{lastTwittedJobIDKey: "10"})

	if got := runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL, saturday()); got.Skipped != "weekend" {
		t.Fatalf("expected weekend skip, got %+v", got)
	}
	if fx.callCount() != 0 {
		t.Fatalf("expected 0 tweets on Saturday, got %d", fx.callCount())
	}

	res := runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL, wednesday())
	if res.Posted != 2 || res.Err != nil {
		t.Fatalf("expected 2 posted on Wednesday, got %+v", res)
	}
	if store.getMeta(lastTwittedJobIDKey) != "12" {
		t.Fatalf("expected last_twitted_job_id=12, got %q", store.getMeta(lastTwittedJobIDKey))
	}
	if store.getMeta(lastTwitterSharedDateKey) != "2026-11-04" {
		t.Fatalf("expected last_twitter_shared_date=2026-11-04, got %q", store.getMeta(lastTwitterSharedDateKey))
	}

	// Second trigger on the same Wednesday is a no-op (once per day).
	if got := runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL, wednesday().Add(2*time.Hour)); got.Skipped != "already-shared-today" {
		t.Fatalf("expected already-shared-today skip, got %+v", got)
	}
	if fx.callCount() != 2 {
		t.Fatalf("expected still 2 tweets after repeat trigger, got %d", fx.callCount())
	}

	// Thursday posts the remaining job (#13).
	res = runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL, thursday())
	if res.Posted != 1 || fx.callCount() != 3 {
		t.Fatalf("expected 1 more tweet on Thursday (3 total), got %+v / %d calls", res, fx.callCount())
	}
	if store.getMeta(lastTwittedJobIDKey) != "13" {
		t.Fatalf("expected last_twitted_job_id=13, got %q", store.getMeta(lastTwittedJobIDKey))
	}
}

func TestRunTwitterSchedulerWeekdayUsesConfiguredTimezone(t *testing.T) {
	fx := newFakeXServer(t, nil, fakeXBehavior{status: http.StatusCreated})
	cfg := testTwitterConfig()
	cfg.TwitterTimezone = "America/New_York"
	svr := testServer(cfg)
	store := newFakeTwitterStore(testJobs(), map[string]string{lastTwittedJobIDKey: "10"})

	// Saturday 02:00 UTC is still Friday 21:00 in New York: posting is allowed.
	res := runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL,
		time.Date(2026, 11, 7, 2, 0, 0, 0, time.UTC))
	if res.Posted != 2 {
		t.Fatalf("expected 2 posts on Friday-evening New York time, got %+v", res)
	}
	// Saturday 10:00 UTC is Saturday 05:00 in New York: skip.
	store2 := newFakeTwitterStore(testJobs(), map[string]string{lastTwittedJobIDKey: "10"})
	if got := runTwitterScheduler(svr, store2, fx.server.Client(), fx.server.URL, saturday()); got.Skipped != "weekend" {
		t.Fatalf("expected weekend skip in New York time, got %+v", got)
	}
}

func TestRunTwitterSchedulerMissingCursorStartsAtZero(t *testing.T) {
	fx := newFakeXServer(t, nil, fakeXBehavior{status: http.StatusCreated})
	svr := testServer(testTwitterConfig())
	store := newFakeTwitterStore(testJobs(), nil) // no meta rows at all
	res := runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL, wednesday())
	if res.Posted != 2 || res.Err != nil {
		t.Fatalf("expected 2 posted from an empty cursor, got %+v", res)
	}
}

func TestRunTwitterSchedulerPartialFailureAbortsAndRetries(t *testing.T) {
	fx := newFakeXServer(t, map[string]fakeXBehavior{
		"go-backend-beta":   {status: http.StatusCreated},
		"go-platform-gamma": {status: http.StatusInternalServerError, body: `{"title":"Internal Server Error"}`},
	}, fakeXBehavior{status: http.StatusCreated})
	cfg := testTwitterConfig()
	svr := testServer(cfg)
	store := newFakeTwitterStore(testJobs(), map[string]string{lastTwittedJobIDKey: "10"})

	res := runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL, wednesday())
	if res.Posted != 1 || res.Failed != 1 || res.Err == nil {
		t.Fatalf("expected 1 posted + 1 failed on 500, got %+v", res)
	}
	if fx.callCount() != 2 {
		t.Fatalf("expected 2 calls (job 13 never attempted after abort), got %d", fx.callCount())
	}
	// Cursor advanced past the posted job only, so job 12 is retried.
	if store.getMeta(lastTwittedJobIDKey) != "11" {
		t.Fatalf("expected last_twitted_job_id=11, got %q", store.getMeta(lastTwittedJobIDKey))
	}
	// Next day the failed job succeeds; the day marker from Wednesday does
	// not block Thursday, which posts the two pending jobs (12 and 13).
	fx2 := newFakeXServer(t, nil, fakeXBehavior{status: http.StatusCreated})
	res = runTwitterScheduler(svr, store, fx2.server.Client(), fx2.server.URL, thursday())
	if res.Posted != 2 || res.Err != nil {
		t.Fatalf("expected failed job to post on Thursday, got %+v", res)
	}
	if store.getMeta(lastTwittedJobIDKey) != "13" {
		t.Fatalf("expected last_twitted_job_id=13, got %q", store.getMeta(lastTwittedJobIDKey))
	}
}

func TestRunTwitterSchedulerDuplicateSkipsAndAdvances(t *testing.T) {
	fx := newFakeXServer(t, map[string]fakeXBehavior{
		"go-backend-beta": {status: http.StatusForbidden, body: `{"errors":[{"detail":"You are not allowed to create a Tweet with duplicate content.","title":"Forbidden"}]}`},
	}, fakeXBehavior{status: http.StatusCreated})
	svr := testServer(testTwitterConfig())
	store := newFakeTwitterStore(testJobs(), map[string]string{lastTwittedJobIDKey: "10"})
	res := runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL, wednesday())
	if res.Posted != 1 || res.SkippedJobs != 1 || res.Err != nil {
		t.Fatalf("expected duplicate skipped and next job posted, got %+v", res)
	}
	if store.getMeta(lastTwittedJobIDKey) != "12" {
		t.Fatalf("expected cursor to advance past the duplicate to 12, got %q", store.getMeta(lastTwittedJobIDKey))
	}
}

func TestRunTwitterSchedulerRateLimitAbortsWithRetryAfter(t *testing.T) {
	fx := newFakeXServer(t, map[string]fakeXBehavior{
		"go-backend-beta": {status: http.StatusTooManyRequests, body: `{"title":"Too Many Requests"}`, retryAfter: "120"},
	}, fakeXBehavior{status: http.StatusCreated})
	svr := testServer(testTwitterConfig())
	store := newFakeTwitterStore(testJobs(), map[string]string{lastTwittedJobIDKey: "10"})
	res := runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL, wednesday())
	if res.Posted != 0 || res.Failed != 1 || res.RetryAfter != 120*time.Second {
		t.Fatalf("expected abort with Retry-After 120s, got %+v", res)
	}
	if fx.callCount() != 1 {
		t.Fatalf("expected only 1 call (no further jobs after 429), got %d", fx.callCount())
	}
	if store.getMeta(lastTwittedJobIDKey) != "10" {
		t.Fatalf("expected cursor unchanged at 10 after 429, got %q", store.getMeta(lastTwittedJobIDKey))
	}
	if store.getMeta(lastTwitterSharedDateKey) != "" {
		t.Fatalf("expected no shared-date recorded after a fully failed run")
	}
}

func TestRunTwitterSchedulerAuthFailureAborts(t *testing.T) {
	fx := newFakeXServer(t, nil, fakeXBehavior{status: http.StatusUnauthorized, body: `{"title":"Unauthorized"}`})
	svr := testServer(testTwitterConfig())
	store := newFakeTwitterStore(testJobs(), map[string]string{lastTwittedJobIDKey: "10"})
	res := runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL, wednesday())
	if res.Posted != 0 || res.Failed != 1 || res.Err == nil {
		t.Fatalf("expected auth failure to abort the run, got %+v", res)
	}
	var postErr *xPostError
	if !errors.As(res.Err, &postErr) || !postErr.Auth {
		t.Fatalf("expected classified auth error, got %v", res.Err)
	}
	if store.getMeta(lastTwittedJobIDKey) != "10" {
		t.Fatalf("expected cursor unchanged at 10 after auth failure, got %q", store.getMeta(lastTwittedJobIDKey))
	}
}

func TestRunTwitterSchedulerDryRunPostsNothing(t *testing.T) {
	fx := newFakeXServer(t, nil, fakeXBehavior{status: http.StatusCreated})
	cfg := testTwitterConfig()
	cfg.TwitterDryRun = true
	cfg.TwitterClientKey = "" // dry-run must work without credentials
	cfg.TwitterClientSecret = ""
	cfg.TwitterAccessToken = ""
	cfg.TwitterAccessTokenSecret = ""
	svr := testServer(cfg)
	store := newFakeTwitterStore(testJobs(), map[string]string{lastTwittedJobIDKey: "10"})
	res := runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL, wednesday())
	if res.Skipped != "dry-run" || res.Err != nil {
		t.Fatalf("expected dry-run result, got %+v", res)
	}
	if fx.callCount() != 0 {
		t.Fatalf("expected 0 HTTP requests in dry-run, got %d", fx.callCount())
	}
	if store.getMeta(lastTwittedJobIDKey) != "10" || store.getMeta(lastTwitterSharedDateKey) != "" {
		t.Fatalf("expected dry-run to leave cursor and shared date untouched")
	}
}

func TestRunTwitterSchedulerNotConfigured(t *testing.T) {
	fx := newFakeXServer(t, nil, fakeXBehavior{status: http.StatusCreated})
	cfg := testTwitterConfig()
	cfg.TwitterClientKey = ""
	cfg.TwitterClientSecret = ""
	cfg.TwitterAccessToken = ""
	cfg.TwitterAccessTokenSecret = ""
	svr := testServer(cfg)
	store := newFakeTwitterStore(testJobs(), map[string]string{lastTwittedJobIDKey: "10"})
	res := runTwitterScheduler(svr, store, fx.server.Client(), fx.server.URL, wednesday())
	if res.Skipped != "not-configured" || res.Err == nil {
		t.Fatalf("expected not-configured result, got %+v", res)
	}
	if fx.callCount() != 0 {
		t.Fatalf("expected 0 HTTP requests when not configured, got %d", fx.callCount())
	}
}

func TestLastTwitterJobID(t *testing.T) {
	store := newFakeTwitterStore(nil, nil)
	if id, err := lastTwitterJobID(store); id != 0 || err != nil {
		t.Fatalf("missing cursor should be 0, got %d / %v", id, err)
	}
	store.SetValue(lastTwittedJobIDKey, " 42 ")
	if id, err := lastTwitterJobID(store); id != 42 || err != nil {
		t.Fatalf("expected 42, got %d / %v", id, err)
	}
	store.SetValue(lastTwittedJobIDKey, "not-a-number")
	if _, err := lastTwitterJobID(store); err == nil {
		t.Fatalf("expected error for a non-numeric cursor")
	}
}
