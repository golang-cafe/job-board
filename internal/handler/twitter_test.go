package handler

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/server"
)

type mockJobRow struct {
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

type mockDBState struct {
	mu   sync.Mutex
	meta map[string]string
	jobs []mockJobRow
}

var (
	registerMockDriverOnce sync.Once
	activeMockState        *mockDBState
)

type mockSQLDriver struct{}

func (d *mockSQLDriver) Open(name string) (driver.Conn, error) {
	return &mockSQLConn{}, nil
}

type mockSQLConn struct{}

func (c *mockSQLConn) Prepare(query string) (driver.Stmt, error) {
	return &mockSQLStmt{query: query}, nil
}
func (c *mockSQLConn) Close() error              { return nil }
func (c *mockSQLConn) Begin() (driver.Tx, error) { return &mockSQLTx{}, nil }

type mockSQLTx struct{}

func (t *mockSQLTx) Commit() error   { return nil }
func (t *mockSQLTx) Rollback() error { return nil }

type mockSQLStmt struct {
	query string
}

func (s *mockSQLStmt) Close() error  { return nil }
func (s *mockSQLStmt) NumInput() int { return -1 }

func (s *mockSQLStmt) Exec(args []driver.Value) (driver.Result, error) {
	activeMockState.mu.Lock()
	defer activeMockState.mu.Unlock()
	if strings.Contains(s.query, "INSERT INTO meta") && len(args) >= 2 {
		k, _ := args[0].(string)
		v, _ := args[1].(string)
		activeMockState.meta[k] = v
	} else if strings.Contains(s.query, "UPDATE meta") && len(args) >= 2 {
		v, _ := args[0].(string)
		k, _ := args[1].(string)
		activeMockState.meta[k] = v
	}
	return driver.RowsAffected(1), nil
}

func (s *mockSQLStmt) Query(args []driver.Value) (driver.Rows, error) {
	activeMockState.mu.Lock()
	defer activeMockState.mu.Unlock()
	if strings.Contains(s.query, "SELECT value FROM meta") {
		key, _ := args[0].(string)
		val, ok := activeMockState.meta[key]
		if !ok {
			return &mockSQLRows{cols: []string{"value"}}, nil
		}
		return &mockSQLRows{
			cols: []string{"value"},
			rows: [][]driver.Value{{val}},
		}, nil
	}
	if strings.Contains(s.query, "FROM job WHERE id > $1") {
		var minID int64
		var maxRows int64 = 10
		if len(args) >= 1 {
			if v, ok := args[0].(int64); ok {
				minID = v
			}
		}
		if len(args) >= 2 {
			if v, ok := args[1].(int64); ok {
				maxRows = v
			}
		}
		var out [][]driver.Value
		for _, j := range activeMockState.jobs {
			if j.id > minID && int64(len(out)) < maxRows {
				out = append(out, []driver.Value{
					j.id, j.title, j.company, j.salary, j.location, j.slug, j.currency, j.iconID, j.externalID, j.period,
				})
			}
		}
		return &mockSQLRows{
			cols: []string{"id", "job_title", "company", "salary_range", "location", "slug", "salary_currency", "company_icon_image_id", "external_id", "salary_period"},
			rows: out,
		}, nil
	}
	return &mockSQLRows{}, nil
}

type mockSQLRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (r *mockSQLRows) Columns() []string { return r.cols }
func (r *mockSQLRows) Close() error      { return nil }
func (r *mockSQLRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func newMockDB(t *testing.T, state *mockDBState) *sql.DB {
	t.Helper()
	registerMockDriverOnce.Do(func() {
		sql.Register("twitter_mock_db", &mockSQLDriver{})
	})
	activeMockState = state
	db, err := sql.Open("twitter_mock_db", "mock")
	if err != nil {
		t.Fatalf("failed to open mock db: %v", err)
	}
	return db
}

func TestIsWeekday(t *testing.T) {
	monday := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	friday := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	saturday := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

	if !isWeekday(monday) {
		t.Fatalf("expected Monday to be weekday")
	}
	if !isWeekday(friday) {
		t.Fatalf("expected Friday to be weekday")
	}
	if isWeekday(saturday) {
		t.Fatalf("expected Saturday to not be weekday")
	}
	if isWeekday(sunday) {
		t.Fatalf("expected Sunday to not be weekday")
	}
}

func TestFormatTweetText_StandardAndTruncation(t *testing.T) {
	cfg := config.Config{
		SiteJobCategory: "golang",
		URLProtocol:     "https://",
		SiteHost:        "golang.cafe",
	}
	shortJob := job.JobPost{
		JobTitle:    "Senior Go Engineer",
		Company:     "Acme Corp",
		Location:    "Remote",
		SalaryRange: "$150k - $180k",
		Slug:        "senior-go-engineer-acme-corp",
	}
	txt := formatTweetText(shortJob, cfg)
	if !strings.Contains(txt, "#golang #golangjobs") {
		t.Fatalf("expected hashtags in tweet, got: %s", txt)
	}
	if !strings.HasSuffix(txt, "https://golang.cafe/job/senior-go-engineer-acme-corp") {
		t.Fatalf("expected job URL suffix, got: %s", txt)
	}

	longJob := job.JobPost{
		JobTitle:    strings.Repeat("Principal Distributed Systems Go Architect ", 8),
		Company:     "CloudScale Global Infrastructure Inc",
		Location:    "Remote (Worldwide / US / EU)",
		SalaryRange: "$200k - $260k",
		Slug:        "principal-distributed-systems-go-architect",
	}
	longTxt := formatTweetText(longJob, cfg)
	if utf8.RuneCountInString(longTxt) > maxTweetLength {
		t.Fatalf("expected rune count <= %d, got %d", maxTweetLength, utf8.RuneCountInString(longTxt))
	}
	if !strings.HasSuffix(longTxt, "https://golang.cafe/job/principal-distributed-systems-go-architect") {
		t.Fatalf("expected truncated tweet to preserve job URL suffix, got: %s", longTxt)
	}
}

func TestBuildOAuth1Header(t *testing.T) {
	header := buildOAuth1Header(
		"POST",
		"https://api.twitter.com/2/tweets",
		"ckey",
		"csecret",
		"atoken",
		"asecret",
		"fixednonce123",
		"1700000000",
	)
	if !strings.HasPrefix(header, "OAuth ") {
		t.Fatalf("expected OAuth prefix, got: %s", header)
	}
	for _, part := range []string{
		`oauth_consumer_key="ckey"`,
		`oauth_nonce="fixednonce123"`,
		`oauth_signature_method="HMAC-SHA1"`,
		`oauth_timestamp="1700000000"`,
		`oauth_token="atoken"`,
		`oauth_version="1.0"`,
		`oauth_signature="`,
	} {
		if !strings.Contains(header, part) {
			t.Fatalf("expected header to contain %s, got: %s", part, header)
		}
	}
}

func TestRunTwitterScheduler_WeekdayAndWeekend(t *testing.T) {
	var postedTweets []string
	var authHeaders []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		var payload tweetV2Request
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode tweet payload: %v", err)
		}
		postedTweets = append(postedTweets, payload.Text)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"123","text":"ok"}}`))
	}))
	defer ts.Close()

	state := &mockDBState{
		meta: map[string]string{"last_twitted_job_id": "10"},
		jobs: []mockJobRow{
			{id: 11, title: "Go Backend Engineer", company: "Beta", salary: "$140k", location: "Remote", slug: "go-backend-beta"},
			{id: 12, title: "Go Platform Engineer", company: "Gamma", salary: "$160k", location: "Berlin", slug: "go-platform-gamma"},
		},
	}
	db := newMockDB(t, state)
	defer db.Close()

	cfg := config.Config{
		TwitterJobsToPost:        2,
		TwitterClientKey:         "key",
		TwitterClientSecret:      "secret",
		TwitterAccessToken:       "token",
		TwitterAccessTokenSecret: "tokensecret",
		SiteJobCategory:          "golang",
		URLProtocol:              "https://",
		SiteHost:                 "golang.cafe",
	}
	svr := server.NewServer(cfg, db, nil, nil, nil, nil)
	jobRepo := job.NewRepository(db)

	// 1. Weekend should skip posting completely
	saturday := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	runTwitterScheduler(svr, jobRepo, ts.Client(), ts.URL, saturday)
	if len(postedTweets) != 0 {
		t.Fatalf("expected 0 tweets on Saturday, got %d", len(postedTweets))
	}

	// 2. Weekday should post both jobs to Twitter API v2 and update last_twitted_job_id to 12
	wednesday := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	runTwitterScheduler(svr, jobRepo, ts.Client(), ts.URL, wednesday)
	if len(postedTweets) != 2 {
		t.Fatalf("expected 2 tweets on Wednesday, got %d", len(postedTweets))
	}
	if !strings.HasPrefix(authHeaders[0], "OAuth ") {
		t.Fatalf("expected OAuth 1.0a Authorization header, got %s", authHeaders[0])
	}
	if state.meta["last_twitted_job_id"] != "12" {
		t.Fatalf("expected last_twitted_job_id to be updated to 12, got %s", state.meta["last_twitted_job_id"])
	}
}
