package handler

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

type twitterMockJobRow struct {
	id         int64
	title      string
	company    string
	salary     string
	location   string
	slug       string
	currency   string
	iconID     string
	externalID string
	period     string
}

type twitterMockDBState struct {
	mu   sync.Mutex
	meta map[string]string
	jobs []twitterMockJobRow
}

var (
	registerTwitterMockDriverOnce sync.Once
	twitterMockStates             sync.Map
)

type twitterMockSQLDriver struct{}

func (d *twitterMockSQLDriver) Open(dsn string) (driver.Conn, error) {
	val, ok := twitterMockStates.Load(dsn)
	if !ok {
		return nil, fmt.Errorf("unknown mock dsn: %s", dsn)
	}
	return &twitterMockSQLConn{state: val.(*twitterMockDBState)}, nil
}

type twitterMockSQLConn struct {
	state *twitterMockDBState
}

func (c *twitterMockSQLConn) Prepare(query string) (driver.Stmt, error) {
	return &twitterMockSQLStmt{query: query, state: c.state}, nil
}
func (c *twitterMockSQLConn) Close() error              { return nil }
func (c *twitterMockSQLConn) Begin() (driver.Tx, error) { return nil, nil }

type twitterMockSQLStmt struct {
	query string
	state *twitterMockDBState
}

func (s *twitterMockSQLStmt) Close() error  { return nil }
func (s *twitterMockSQLStmt) NumInput() int { return -1 }

func (s *twitterMockSQLStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	q := strings.ToLower(s.query)
	if strings.Contains(q, "insert into meta") {
		k := fmt.Sprint(args[0])
		v := fmt.Sprint(args[1])
		s.state.meta[k] = v
	} else if strings.Contains(q, "update meta") {
		v := fmt.Sprint(args[0])
		k := fmt.Sprint(args[1])
		s.state.meta[k] = v
	}
	return driver.RowsAffected(1), nil
}

func (s *twitterMockSQLStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	q := strings.ToLower(s.query)
	if strings.Contains(q, "select value from meta") {
		key := fmt.Sprint(args[0])
		val, ok := s.state.meta[key]
		if !ok {
			return &twitterMockRows{cols: []string{"value"}}, nil
		}
		return &twitterMockRows{
			cols: []string{"value"},
			rows: [][]driver.Value{{val}},
		}, nil
	}
	if strings.Contains(q, "from job") {
		minID, _ := strconv.ParseInt(fmt.Sprint(args[0]), 10, 64)
		limit, _ := strconv.Atoi(fmt.Sprint(args[1]))
		var out [][]driver.Value
		for _, j := range s.state.jobs {
			if j.id > minID {
				out = append(out, []driver.Value{
					j.id, j.title, j.company, j.salary, j.location, j.slug,
					j.currency, j.iconID, j.externalID, j.period,
				})
				if len(out) >= limit {
					break
				}
			}
		}
		return &twitterMockRows{
			cols: []string{"id", "job_title", "company", "salary_range", "location", "slug", "salary_currency", "company_icon_image_id", "external_id", "salary_period"},
			rows: out,
		}, nil
	}
	return &twitterMockRows{}, nil
}

type twitterMockRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (r *twitterMockRows) Columns() []string { return r.cols }
func (r *twitterMockRows) Close() error      { return nil }
func (r *twitterMockRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func newTwitterMockDB(t *testing.T, state *twitterMockDBState) *sql.DB {
	t.Helper()
	registerTwitterMockDriverOnce.Do(func() {
		sql.Register("twitter_mock_db", &twitterMockSQLDriver{})
	})
	dsn := fmt.Sprintf("dsn_%d", time.Now().UnixNano())
	twitterMockStates.Store(dsn, state)
	t.Cleanup(func() { twitterMockStates.Delete(dsn) })
	db, err := sql.Open("twitter_mock_db", dsn)
	if err != nil {
		t.Fatalf("failed to open mock db: %v", err)
	}
	return db
}

func TestBuildOAuth1HeaderRFC5849(t *testing.T) {
	hdr := buildOAuth1Header(
		http.MethodPost,
		"https://api.twitter.com/2/tweets",
		"ckey",
		"csecret",
		"atoken",
		"asecret",
		"nonce123",
		"1700000000",
	)
	if !strings.HasPrefix(hdr, "OAuth ") {
		t.Fatalf("expected OAuth prefix, got %q", hdr)
	}
	for _, field := range []string{
		`oauth_consumer_key="ckey"`,
		`oauth_nonce="nonce123"`,
		`oauth_signature_method="HMAC-SHA1"`,
		`oauth_timestamp="1700000000"`,
		`oauth_token="atoken"`,
		`oauth_version="1.0"`,
		`oauth_signature="`,
	} {
		if !strings.Contains(hdr, field) {
			t.Fatalf("expected header %q to contain %q", hdr, field)
		}
	}
}

func TestBuildTwitterAuthorizationHeaderBearerFallback(t *testing.T) {
	cfg := config.Config{TwitterBearerToken: "v2-bearer-secret"}
	hdr := buildTwitterAuthorizationHeader(http.MethodPost, defaultTwitterV2Endpoint, cfg, time.Now())
	if hdr != "Bearer v2-bearer-secret" {
		t.Fatalf("expected Bearer fallback header, got %q", hdr)
	}
}

func TestFormatTweetTextRuneSafeTruncation(t *testing.T) {
	cfg := config.Config{
		SiteJobCategory: "golang",
		URLProtocol:     "https://",
		SiteHost:        "golang.cafe",
	}
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
	if utf8.RuneCountInString(txt) > maxTweetLength {
		t.Fatalf("expected <= %d runes, got %d", maxTweetLength, utf8.RuneCountInString(txt))
	}
	if !strings.HasSuffix(txt, "https://golang.cafe/job/senior-go-architect-acme") {
		t.Fatalf("expected canonical job link preserved at end of tweet, got %q", txt)
	}
}

func TestRunTwitterSchedulerWeekdayOncePerDayAndCap(t *testing.T) {
	var (
		mu           sync.Mutex
		postedTweets []string
		authHeaders  []string
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		var payload tweetV2Request
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode tweet payload: %v", err)
		}
		mu.Lock()
		postedTweets = append(postedTweets, payload.Text)
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"123","text":"ok"}}`))
	}))
	defer ts.Close()

	state := &twitterMockDBState{
		meta: map[string]string{lastTwittedJobIDKey: "10"},
		jobs: []twitterMockJobRow{
			{id: 11, title: "Go Backend Engineer", company: "Beta", salary: "$140k", location: "Remote", slug: "go-backend-beta"},
			{id: 12, title: "Go Platform Engineer", company: "Gamma", salary: "$160k", location: "Berlin", slug: "go-platform-gamma"},
			{id: 13, title: "Go Runtime Engineer", company: "Delta", salary: "$175k", location: "London", slug: "go-runtime-delta"},
		},
	}
	db := newTwitterMockDB(t, state)
	defer db.Close()

	cfg := config.Config{
		TwitterJobsToPost:        5, // should be capped to 2 per day
		TwitterClientKey:         "key",
		TwitterClientSecret:      "secret",
		TwitterAccessToken:       "token",
		TwitterAccessTokenSecret: "tokensecret",
		SiteJobCategory:          "golang",
		URLProtocol:              "https://",
		SiteHost:                 "golang.cafe",
	}
	svr := server.NewServer(cfg, db, nil, nil, email.Client{}, nil)
	jobRepo := job.NewRepository(db)

	// 1. Saturday & Sunday skip posting completely
	saturday := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	runTwitterScheduler(svr, jobRepo, ts.Client(), ts.URL, saturday)
	runTwitterScheduler(svr, jobRepo, ts.Client(), ts.URL, sunday)
	if len(postedTweets) != 0 {
		t.Fatalf("expected 0 tweets on weekend, got %d", len(postedTweets))
	}

	// 2. Wednesday posts at most 2 jobs and records last_twitted_job_id=12 and last_twitter_shared_date=2026-11-04
	wednesday := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	runTwitterScheduler(svr, jobRepo, ts.Client(), ts.URL, wednesday)
	if len(postedTweets) != 2 {
		t.Fatalf("expected 2 tweets on Wednesday (daily cap of 2), got %d", len(postedTweets))
	}
	if !strings.HasPrefix(authHeaders[0], "OAuth ") {
		t.Fatalf("expected OAuth 1.0a Authorization header, got %q", authHeaders[0])
	}
	if state.meta[lastTwittedJobIDKey] != "12" {
		t.Fatalf("expected last_twitted_job_id=12, got %q", state.meta[lastTwittedJobIDKey])
	}
	if state.meta[lastTwitterSharedDateKey] != "2026-11-04" {
		t.Fatalf("expected last_twitter_shared_date=2026-11-04, got %q", state.meta[lastTwitterSharedDateKey])
	}

	// 3. Second call on the same Wednesday is skipped (once-per-day enforcement)
	runTwitterScheduler(svr, jobRepo, ts.Client(), ts.URL, wednesday.Add(2*time.Hour))
	if len(postedTweets) != 2 {
		t.Fatalf("expected second run on same weekday to be skipped, got %d total tweets", len(postedTweets))
	}

	// 4. Following weekday (Thursday) posts the remaining job (#13)
	thursday := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	runTwitterScheduler(svr, jobRepo, ts.Client(), ts.URL, thursday)
	if len(postedTweets) != 3 {
		t.Fatalf("expected 3 total tweets after Thursday run, got %d", len(postedTweets))
	}
	if state.meta[lastTwittedJobIDKey] != "13" {
		t.Fatalf("expected last_twitted_job_id=13, got %q", state.meta[lastTwittedJobIDKey])
	}
}
