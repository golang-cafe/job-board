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
	"github.com/gorilla/mux"
)

type mockMetaJobState struct {
	mu   sync.Mutex
	meta map[string]string
	jobs []*job.JobPost
}

var (
	registerMockDriverOnce sync.Once
	activeMockState        *mockMetaJobState
	activeMockStateMu      sync.Mutex
)

func setMockDBState(st *mockMetaJobState) *sql.DB {
	registerMockDriverOnce.Do(func() {
		sql.Register("mock_job_board_db", &mockSQLDriver{})
	})
	activeMockStateMu.Lock()
	activeMockState = st
	activeMockStateMu.Unlock()
	db, err := sql.Open("mock_job_board_db", "test")
	if err != nil {
		panic(err)
	}
	return db
}

type mockSQLDriver struct{}

func (d *mockSQLDriver) Open(name string) (driver.Conn, error) {
	return &mockSQLConn{}, nil
}

type mockSQLConn struct{}

func (c *mockSQLConn) Prepare(query string) (driver.Stmt, error) {
	return &mockSQLStmt{query: query}, nil
}

func (c *mockSQLConn) Close() error              { return nil }
func (c *mockSQLConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("tx not supported") }

type mockSQLStmt struct {
	query string
}

func (s *mockSQLStmt) Close() error  { return nil }
func (s *mockSQLStmt) NumInput() int { return -1 }

func (s *mockSQLStmt) Exec(args []driver.Value) (driver.Result, error) {
	activeMockStateMu.Lock()
	st := activeMockState
	activeMockStateMu.Unlock()
	st.mu.Lock()
	defer st.mu.Unlock()

	q := strings.ToLower(strings.TrimSpace(s.query))
	switch {
	case strings.HasPrefix(q, "insert into meta"):
		key := fmt.Sprint(args[0])
		val := fmt.Sprint(args[1])
		st.meta[key] = val
		return driver.RowsAffected(1), nil
	case strings.HasPrefix(q, "update meta set value"):
		val := fmt.Sprint(args[0])
		key := fmt.Sprint(args[1])
		st.meta[key] = val
		return driver.RowsAffected(1), nil
	default:
		return driver.RowsAffected(0), fmt.Errorf("unexpected exec query: %s", s.query)
	}
}

func (s *mockSQLStmt) Query(args []driver.Value) (driver.Rows, error) {
	activeMockStateMu.Lock()
	st := activeMockState
	activeMockStateMu.Unlock()
	st.mu.Lock()
	defer st.mu.Unlock()

	q := strings.ToLower(strings.TrimSpace(s.query))
	switch {
	case strings.HasPrefix(q, "select value from meta"):
		key := fmt.Sprint(args[0])
		val, ok := st.meta[key]
		if !ok {
			return &mockSQLRows{cols: []string{"value"}, rows: nil}, nil
		}
		return &mockSQLRows{
			cols: []string{"value"},
			rows: [][]driver.Value{{val}},
		}, nil
	case strings.HasPrefix(q, "select id, job_title, company"):
		minID, _ := strconv.Atoi(fmt.Sprint(args[0]))
		limit, _ := strconv.Atoi(fmt.Sprint(args[1]))
		var matched [][]driver.Value
		for _, j := range st.jobs {
			if j.ID > minID {
				matched = append(matched, []driver.Value{
					int64(j.ID),
					j.JobTitle,
					j.Company,
					j.SalaryRange,
					j.Location,
					j.Slug,
					j.SalaryCurrency,
					j.CompanyIconID,
					j.ExternalID,
					j.SalaryPeriod,
				})
				if len(matched) >= limit {
					break
				}
			}
		}
		return &mockSQLRows{
			cols: []string{"id", "job_title", "company", "salary_range", "location", "slug", "salary_currency", "company_icon_image_id", "external_id", "salary_period"},
			rows: matched,
		}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", s.query)
	}
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

func TestIsWeekday(t *testing.T) {
	tests := []struct {
		name string
		day  time.Time
		want bool
	}{
		{"Monday", time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC), true},
		{"Tuesday", time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC), true},
		{"Wednesday", time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC), true},
		{"Thursday", time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC), true},
		{"Friday", time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC), true},
		{"Saturday", time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC), false},
		{"Sunday", time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isWeekday(tc.day); got != tc.want {
				t.Fatalf("isWeekday(%s) = %v, want %v", tc.day.Weekday(), got, tc.want)
			}
		})
	}
}

func TestFormatTweetText(t *testing.T) {
	cfg := config.Config{
		SiteJobCategory: "golang",
		URLProtocol:     "https://",
		SiteHost:        "golang.cafe",
	}
	tests := []struct {
		name         string
		jobPost      *job.JobPost
		wantContains []string
		wantMaxRunes int
	}{
		{
			name: "standard job post",
			jobPost: &job.JobPost{
				JobTitle:    "Senior Backend Engineer",
				Company:     "Acme Corp",
				Location:    "Remote",
				SalaryRange: "$140k - $180k",
				Slug:        "senior-backend-engineer-acme-corp",
			},
			wantContains: []string{
				"Senior Backend Engineer with Acme Corp - Remote | $140k - $180k",
				"#golang #golangjobs",
				"https://golang.cafe/job/senior-backend-engineer-acme-corp",
			},
			wantMaxRunes: 280,
		},
		{
			name: "overlong job title and location gets truncated within 280 runes while preserving hashtags and URL",
			jobPost: &job.JobPost{
				JobTitle:    strings.Repeat("Principal Distributed Systems Go Architect ", 6),
				Company:     "Global Cloud Infrastructure Inc",
				Location:    "Remote (US / EU / UK / APAC Timezones)",
				SalaryRange: "$200k - $260k",
				Slug:        "principal-distributed-systems-go-architect",
			},
			wantContains: []string{
				"...",
				"#golang #golangjobs",
				"https://golang.cafe/job/principal-distributed-systems-go-architect",
			},
			wantMaxRunes: 280,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatTweetText(tc.jobPost, cfg)
			if runeCount := utf8.RuneCountInString(got); runeCount > tc.wantMaxRunes {
				t.Fatalf("formatTweetText rune count = %d, want <= %d; text=%q", runeCount, tc.wantMaxRunes, got)
			}
			for _, sub := range tc.wantContains {
				if !strings.Contains(got, sub) {
					t.Errorf("formatTweetText(%q) missing expected substring %q", got, sub)
				}
			}
		})
	}
}

func TestBuildOAuth1Header(t *testing.T) {
	header, err := buildOAuth1Header(
		http.MethodPost,
		"https://api.twitter.com/2/tweets",
		"consumer-key-123",
		"consumer-secret-456",
		"access-token-789",
		"access-secret-012",
		"fixednonce123456",
		"1700000000",
	)
	if err != nil {
		t.Fatalf("buildOAuth1Header returned unexpected error: %v", err)
	}
	wantParts := []string{
		`OAuth `,
		`oauth_consumer_key="consumer-key-123"`,
		`oauth_nonce="fixednonce123456"`,
		`oauth_signature="`,
		`oauth_signature_method="HMAC-SHA1"`,
		`oauth_timestamp="1700000000"`,
		`oauth_token="access-token-789"`,
		`oauth_version="1.0"`,
	}
	for _, part := range wantParts {
		if !strings.Contains(header, part) {
			t.Errorf("OAuth header %q missing part %q", header, part)
		}
	}
	// Deterministic check: calling with identical inputs produces identical signature
	header2, err := buildOAuth1Header(
		http.MethodPost,
		"https://api.twitter.com/2/tweets",
		"consumer-key-123",
		"consumer-secret-456",
		"access-token-789",
		"access-secret-012",
		"fixednonce123456",
		"1700000000",
	)
	if err != nil || header != header2 {
		t.Fatalf("deterministic OAuth signature mismatch: %q vs %q (err=%v)", header, header2, err)
	}
}

func TestPostTweetV2(t *testing.T) {
	tests := []struct {
		name           string
		cfg            config.Config
		tweetText      string
		serverStatus   int
		serverResponse string
		wantAuthPrefix string
		wantErr        bool
	}{
		{
			name: "OAuth 1.0a 4-key user context succeeds on 201 Created",
			cfg: config.Config{
				TwitterClientKey:         "ck_test",
				TwitterClientSecret:      "cs_test",
				TwitterAccessToken:       "at_test",
				TwitterAccessTokenSecret: "ats_test",
			},
			tweetText:      "Backend Engineer with Google - London | $100k - $120k\n\n#golang #golangjobs\n\nhttps://golang.cafe/job/backend-engineer",
			serverStatus:   http.StatusCreated,
			serverResponse: `{"data":{"id":"1849000000000000001","text":"ok"}}`,
			wantAuthPrefix: "OAuth ",
			wantErr:        false,
		},
		{
			name: "API 401 unauthorized returns descriptive error",
			cfg: config.Config{
				TwitterClientKey:         "ck_test",
				TwitterClientSecret:      "cs_test",
				TwitterAccessToken:       "at_test",
				TwitterAccessTokenSecret: "ats_test",
			},
			tweetText:      "Test tweet",
			serverStatus:   http.StatusUnauthorized,
			serverResponse: `{"title":"Unauthorized","status":401,"detail":"Unauthorized"}`,
			wantAuthPrefix: "OAuth ",
			wantErr:        true,
		},
		{
			name:           "missing credentials returns error before request",
			cfg:            config.Config{},
			tweetText:      "Test tweet",
			serverStatus:   http.StatusCreated,
			serverResponse: `{}`,
			wantErr:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var receivedText string
			var receivedAuth string
			var receivedContentType string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("unexpected method %s, want POST", r.Method)
				}
				if r.URL.Path != "/2/tweets" {
					t.Errorf("unexpected path %s, want /2/tweets", r.URL.Path)
				}
				receivedAuth = r.Header.Get("Authorization")
				receivedContentType = r.Header.Get("Content-Type")
				var payload struct {
					Text string `json:"text"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("invalid JSON body: %v", err)
				}
				receivedText = payload.Text
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.serverStatus)
				_, _ = w.Write([]byte(tc.serverResponse))
			}))
			defer srv.Close()

			err := postTweetV2(srv.Client(), srv.URL+"/2/tweets", tc.cfg, tc.tweetText)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if receivedContentType != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", receivedContentType)
			}
			if !strings.HasPrefix(receivedAuth, tc.wantAuthPrefix) {
				t.Errorf("Authorization header = %q, want prefix %q", receivedAuth, tc.wantAuthPrefix)
			}
			if receivedText != tc.tweetText {
				t.Errorf("decoded tweet text = %q, want %q", receivedText, tc.tweetText)
			}
		})
	}
}

func TestRunTwitterScheduler_WeekdayAndWeekend(t *testing.T) {
	sampleJobs := []*job.JobPost{
		{
			ID:          11,
			JobTitle:    "Go Systems Engineer",
			Company:     "Cloudflare",
			Location:    "Remote",
			SalaryRange: "$150k - $190k",
			Slug:        "go-systems-engineer-cloudflare",
		},
		{
			ID:          12,
			JobTitle:    "Staff Golang Developer",
			Company:     "Stripe",
			Location:    "London, UK",
			SalaryRange: "£120k - £150k",
			Slug:        "staff-golang-developer-stripe",
		},
		{
			ID:          13,
			JobTitle:    "Platform Engineer",
			Company:     "Monzo",
			Location:    "London, UK",
			SalaryRange: "£100k - £130k",
			Slug:        "platform-engineer-monzo",
		},
	}

	t.Run("skips execution on Saturday and Sunday without calling Twitter API", func(t *testing.T) {
		apiCalls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiCalls++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"1"}}`))
		}))
		defer srv.Close()

		st := &mockMetaJobState{
			meta: map[string]string{"last_twitted_job_id": "10"},
			jobs: sampleJobs,
		}
		db := setMockDBState(st)
		defer db.Close()

		cfg := config.Config{
			MachineToken:             "secret-machine-token",
			TwitterJobsToPost:        2,
			TwitterClientKey:         "ck",
			TwitterClientSecret:      "cs",
			TwitterAccessToken:       "at",
			TwitterAccessTokenSecret: "ats",
			SiteJobCategory:          "golang",
			URLProtocol:              "https://",
			SiteHost:                 "golang.cafe",
		}
		svr := server.NewServer(cfg, db, mux.NewRouter(), nil, email.Client{}, nil)
		jobRepo := job.NewRepository(db)

		saturday := time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC)
		runTwitterScheduler(svr, jobRepo, srv.Client(), srv.URL+"/2/tweets", saturday)

		sunday := time.Date(2026, 10, 25, 10, 0, 0, 0, time.UTC)
		runTwitterScheduler(svr, jobRepo, srv.Client(), srv.URL+"/2/tweets", sunday)

		if apiCalls != 0 {
			t.Fatalf("expected 0 API calls on weekend, got %d", apiCalls)
		}
		if got := st.meta["last_twitted_job_id"]; got != "10" {
			t.Fatalf("last_twitted_job_id = %q, want 10", got)
		}
	})

	t.Run("posts configured number of jobs on weekday and updates last_twitted_job_id", func(t *testing.T) {
		var postedTweets []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Text string `json:"text"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			postedTweets = append(postedTweets, body.Text)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"1"}}`))
		}))
		defer srv.Close()

		st := &mockMetaJobState{
			meta: map[string]string{"last_twitted_job_id": "10"},
			jobs: sampleJobs,
		}
		db := setMockDBState(st)
		defer db.Close()

		cfg := config.Config{
			MachineToken:             "secret-machine-token",
			TwitterJobsToPost:        2,
			TwitterClientKey:         "ck",
			TwitterClientSecret:      "cs",
			TwitterAccessToken:       "at",
			TwitterAccessTokenSecret: "ats",
			SiteJobCategory:          "golang",
			URLProtocol:              "https://",
			SiteHost:                 "golang.cafe",
		}
		svr := server.NewServer(cfg, db, mux.NewRouter(), nil, email.Client{}, nil)
		jobRepo := job.NewRepository(db)

		wednesday := time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)
		runTwitterScheduler(svr, jobRepo, srv.Client(), srv.URL+"/2/tweets", wednesday)

		if len(postedTweets) != 2 {
			t.Fatalf("expected 2 tweets posted on Wednesday, got %d", len(postedTweets))
		}
		if !strings.Contains(postedTweets[0], "Go Systems Engineer with Cloudflare") {
			t.Errorf("first tweet = %q, missing job 11 details", postedTweets[0])
		}
		if !strings.Contains(postedTweets[1], "Staff Golang Developer with Stripe") {
			t.Errorf("second tweet = %q, missing job 12 details", postedTweets[1])
		}
		if got := st.meta["last_twitted_job_id"]; got != "12" {
			t.Fatalf("last_twitted_job_id = %q, want 12", got)
		}
	})

	t.Run("TriggerTwitterScheduler HTTP handler enforces machine token auth", func(t *testing.T) {
		st := &mockMetaJobState{
			meta: map[string]string{"last_twitted_job_id": "12"},
			jobs: nil,
		}
		db := setMockDBState(st)
		defer db.Close()

		cfg := config.Config{
			MachineToken:      "secret-machine-token",
			TwitterJobsToPost: 2,
		}
		svr := server.NewServer(cfg, db, mux.NewRouter(), nil, email.Client{}, nil)
		jobRepo := job.NewRepository(db)
		h := TriggerTwitterScheduler(svr, jobRepo)

		// Unauthorized request
		reqUnauth := httptest.NewRequest(http.MethodPost, "/x/task/twitter-jobs-scheduler", nil)
		recUnauth := httptest.NewRecorder()
		h.ServeHTTP(recUnauth, reqUnauth)
		if recUnauth.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized status = %d, want %d", recUnauth.Code, http.StatusUnauthorized)
		}

		// Authorized request
		reqAuth := httptest.NewRequest(http.MethodPost, "/x/task/twitter-jobs-scheduler", nil)
		reqAuth.Header.Set("x-machine-token", "secret-machine-token")
		recAuth := httptest.NewRecorder()
		h.ServeHTTP(recAuth, reqAuth)
		if recAuth.Code != http.StatusOK {
			t.Fatalf("authorized status = %d, want %d", recAuth.Code, http.StatusOK)
		}
	})
}
