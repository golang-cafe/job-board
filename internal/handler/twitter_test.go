package handler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
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

type mockTwitterStore struct {
	mu       sync.Mutex
	meta     map[string]string
	jobs     []*job.JobPost
	setCalls []string
}

type mockConnector struct {
	store *mockTwitterStore
}

func (c *mockConnector) Connect(_ context.Context) (driver.Conn, error) {
	return &mockConn{store: c.store}, nil
}

func (c *mockConnector) Driver() driver.Driver {
	return &mockDriver{store: c.store}
}

type mockDriver struct {
	store *mockTwitterStore
}

func (d *mockDriver) Open(_ string) (driver.Conn, error) {
	return &mockConn{store: d.store}, nil
}

type mockConn struct {
	store *mockTwitterStore
}

func (c *mockConn) Prepare(query string) (driver.Stmt, error) {
	return &mockStmt{store: c.store, query: query}, nil
}

func (c *mockConn) Close() error {
	return nil
}

func (c *mockConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("transactions not supported")
}

type mockStmt struct {
	store *mockTwitterStore
	query string
}

func (s *mockStmt) Close() error {
	return nil
}

func (s *mockStmt) NumInput() int {
	return -1
}

func (s *mockStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if strings.Contains(s.query, "UPDATE meta SET value") && len(args) == 2 {
		val := fmt.Sprintf("%v", args[0])
		key := fmt.Sprintf("%v", args[1])
		s.store.meta[key] = val
		s.store.setCalls = append(s.store.setCalls, val)
		return driver.RowsAffected(1), nil
	}
	return driver.RowsAffected(0), nil
}

func (s *mockStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	if strings.Contains(s.query, "SELECT value FROM meta") && len(args) == 1 {
		key := fmt.Sprintf("%v", args[0])
		val, ok := s.store.meta[key]
		if !ok {
			return &mockRows{cols: []string{"value"}}, nil
		}
		return &mockRows{
			cols: []string{"value"},
			data: [][]driver.Value{{val}},
		}, nil
	}
	if strings.Contains(s.query, "FROM job WHERE id > $1") && len(args) == 2 {
		minID, err := toInt(args[0])
		if err != nil {
			return nil, err
		}
		limit, err := toInt(args[1])
		if err != nil {
			return nil, err
		}
		rowsData := make([][]driver.Value, 0, limit)
		for _, j := range s.store.jobs {
			if j.ID > minID {
				rowsData = append(rowsData, []driver.Value{
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
				if len(rowsData) >= limit {
					break
				}
			}
		}
		return &mockRows{
			cols: []string{
				"id", "job_title", "company", "salary_range", "location",
				"slug", "salary_currency", "company_icon_image_id", "external_id", "salary_period",
			},
			data: rowsData,
		}, nil
	}
	return &mockRows{}, nil
}

func toInt(v driver.Value) (int, error) {
	switch val := v.(type) {
	case int64:
		return int(val), nil
	case int:
		return val, nil
	case string:
		return strconv.Atoi(val)
	default:
		return 0, fmt.Errorf("unexpected type %T", v)
	}
}

type mockRows struct {
	cols []string
	data [][]driver.Value
	idx  int
}

func (r *mockRows) Columns() []string {
	return r.cols
}

func (r *mockRows) Close() error {
	return nil
}

func (r *mockRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.data) {
		return io.EOF
	}
	copy(dest, r.data[r.idx])
	r.idx++
	return nil
}

func newTestConfig() config.Config {
	return config.Config{
		TwitterJobsToPost:        2,
		TwitterAccessToken:       "access-token",
		TwitterAccessTokenSecret: "access-secret",
		TwitterClientKey:         "consumer-key",
		TwitterClientSecret:      "consumer-secret",
		SiteJobCategory:          "golang",
		URLProtocol:              "https://",
		SiteHost:                 "golang.cafe",
		MachineToken:             "test-machine-token",
	}
}

func newTestServer(store *mockTwitterStore, cfg config.Config) (server.Server, *job.Repository) {
	db := sql.OpenDB(&mockConnector{store: store})
	svr := server.NewServer(cfg, db, mux.NewRouter(), nil, email.Client{}, nil)
	return svr, job.NewRepository(db)
}

func TestIsWeekday(t *testing.T) {
	cases := []struct {
		day  time.Time
		want bool
	}{
		{time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC), true},  // Monday
		{time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC), true},  // Tuesday
		{time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC), true},  // Wednesday
		{time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC), true},  // Thursday
		{time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC), true},  // Friday
		{time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC), false}, // Saturday
		{time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), false},  // Sunday
	}
	for _, tc := range cases {
		if got := isWeekday(tc.day); got != tc.want {
			t.Fatalf("isWeekday(%s) = %v, want %v", tc.day.Weekday(), got, tc.want)
		}
	}
}

func TestBuildOAuth1Header(t *testing.T) {
	header, err := buildOAuth1Header(
		http.MethodPost,
		defaultTwitterV2Endpoint,
		"xvz1evFS4wEEPTGEFPHBog",
		"kAcSOqF21Fu85e7zjz7ZN2U4ZRhfV3WpwPAoE3Z7kBw",
		"370773112-GmHxMAgYyLbNEtIKZeRNFsMKPR9EyMZeS9weJAEb",
		"LswwdoUaIvS8ltyTt5jkRh4J50vUPVVHtR2YPi5kE",
		"kYjzVBB8Y0ZFabxSWbWovY3uYSQ2pTgmZeNu2VS4cg",
		"1318622958",
	)
	if err != nil {
		t.Fatalf("unexpected error building OAuth1 header: %v", err)
	}
	if !strings.HasPrefix(header, "OAuth ") {
		t.Fatalf("expected OAuth prefix, got %q", header)
	}
	for _, field := range []string{
		`oauth_consumer_key="xvz1evFS4wEEPTGEFPHBog"`,
		`oauth_nonce="kYjzVBB8Y0ZFabxSWbWovY3uYSQ2pTgmZeNu2VS4cg"`,
		`oauth_signature_method="HMAC-SHA1"`,
		`oauth_timestamp="1318622958"`,
		`oauth_token="370773112-GmHxMAgYyLbNEtIKZeRNFsMKPR9EyMZeS9weJAEb"`,
		`oauth_version="1.0"`,
		`oauth_signature="`,
	} {
		if !strings.Contains(header, field) {
			t.Fatalf("missing %s in header %s", field, header)
		}
	}
}

func TestFormatTweetText(t *testing.T) {
	cfg := newTestConfig()
	shortJob := &job.JobPost{
		JobTitle:    "Senior Go Engineer",
		Company:     "Acme",
		Location:    "Remote",
		SalaryRange: "$140k - $180k",
		Slug:        "senior-go-engineer-acme",
	}
	got := formatTweetText(shortJob, cfg)
	want := "Senior Go Engineer with Acme - Remote | $140k - $180k\n\n#golang #golangjobs\n\nhttps://golang.cafe/job/senior-go-engineer-acme"
	if got != want {
		t.Fatalf("formatTweetText mismatch:\ngot:  %q\nwant: %q", got, want)
	}

	longJob := &job.JobPost{
		JobTitle:    strings.Repeat("Principal Distributed Systems Go Architect ", 6),
		Company:     "Global Cloud Corp",
		Location:    "Remote (Worldwide)",
		SalaryRange: "$180k - $240k",
		Slug:        "principal-distributed-systems-go-architect",
	}
	truncated := formatTweetText(longJob, cfg)
	if utf8.RuneCountInString(truncated) > maxTweetCharacters {
		t.Fatalf("expected tweet <= %d chars, got %d", maxTweetCharacters, utf8.RuneCountInString(truncated))
	}
	if !strings.HasSuffix(truncated, "\n\n#golang #golangjobs\n\nhttps://golang.cafe/job/principal-distributed-systems-go-architect") {
		t.Fatalf("expected truncated tweet to preserve hashtags and URL suffix, got %q", truncated)
	}
}

func TestRunTwitterSchedulerWeekdayAndWeekend(t *testing.T) {
	var received []string
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "OAuth ") {
			t.Errorf("missing OAuth Authorization header: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %q", r.Header.Get("Content-Type"))
		}
		body, readErr := ioutil.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("failed reading body: %v", readErr)
		}
		var payload tweetV2Request
		if unmarshalErr := json.Unmarshal(body, &payload); unmarshalErr != nil {
			t.Errorf("failed unmarshaling body: %v", unmarshalErr)
		}
		mu.Lock()
		received = append(received, payload.Text)
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		if _, writeErr := w.Write([]byte(`{"data":{"id":"1","text":"ok"}}`)); writeErr != nil {
			t.Errorf("failed writing response: %v", writeErr)
		}
	}))
	defer ts.Close()

	store := &mockTwitterStore{
		meta: map[string]string{lastTwittedJobIDMetaKey: "1"},
		jobs: []*job.JobPost{
			{ID: 1, JobTitle: "Go Dev 1", Company: "C1", Location: "Remote", SalaryRange: "$100k", Slug: "go-dev-1"},
			{ID: 2, JobTitle: "Go Dev 2", Company: "C2", Location: "Remote", SalaryRange: "$120k", Slug: "go-dev-2"},
			{ID: 3, JobTitle: "Go Dev 3", Company: "C3", Location: "Remote", SalaryRange: "$130k", Slug: "go-dev-3"},
			{ID: 4, JobTitle: "Go Dev 4", Company: "C4", Location: "Remote", SalaryRange: "$140k", Slug: "go-dev-4"},
		},
	}
	svr, jobRepo := newTestServer(store, newTestConfig())

	saturday := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	runTwitterScheduler(svr, jobRepo, ts.Client(), ts.URL, saturday)
	mu.Lock()
	if len(received) != 0 {
		t.Fatalf("expected 0 tweets on Saturday, got %d", len(received))
	}
	mu.Unlock()
	store.mu.Lock()
	if got := store.meta[lastTwittedJobIDMetaKey]; got != "1" {
		t.Fatalf("expected last_twitted_job_id to remain 1 on weekend, got %q", got)
	}
	store.mu.Unlock()

	monday := time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)
	runTwitterScheduler(svr, jobRepo, ts.Client(), ts.URL, monday)
	mu.Lock()
	if len(received) != 2 {
		t.Fatalf("expected 2 tweets on Monday, got %d", len(received))
	}
	mu.Unlock()
	store.mu.Lock()
	if got := store.meta[lastTwittedJobIDMetaKey]; got != "3" {
		t.Fatalf("expected last_twitted_job_id to update to 3, got %q", got)
	}
	store.mu.Unlock()
}

func TestTriggerTwitterJobsSchedulerAuth(t *testing.T) {
	store := &mockTwitterStore{
		meta: map[string]string{lastTwittedJobIDMetaKey: "1"},
	}
	svr, jobRepo := newTestServer(store, newTestConfig())
	h := TriggerTwitterJobsScheduler(svr, jobRepo)

	unauthReq := httptest.NewRequest(http.MethodPost, "/x/task/twitter-jobs-scheduler", nil)
	unauthRec := httptest.NewRecorder()
	h.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing machine token, got %d", unauthRec.Code)
	}

	authReq := httptest.NewRequest(http.MethodPost, "/x/task/twitter-jobs-scheduler", nil)
	authReq.Header.Set("x-machine-token", "test-machine-token")
	authRec := httptest.NewRecorder()
	h.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid machine token, got %d", authRec.Code)
	}
}
