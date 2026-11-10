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

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/email"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/server"
)

type linkedinMockJobRow struct {
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

type linkedinMockDBState struct {
	mu   sync.Mutex
	meta map[string]string
	jobs []linkedinMockJobRow
}

var (
	registerLinkedinMockDriverOnce sync.Once
	linkedinMockStates             sync.Map
)

type linkedinMockSQLDriver struct{}

func (d *linkedinMockSQLDriver) Open(dsn string) (driver.Conn, error) {
	val, ok := linkedinMockStates.Load(dsn)
	if !ok {
		return nil, fmt.Errorf("unknown mock dsn: %s", dsn)
	}
	return &linkedinMockSQLConn{state: val.(*linkedinMockDBState)}, nil
}

type linkedinMockSQLConn struct {
	state *linkedinMockDBState
}

func (c *linkedinMockSQLConn) Prepare(query string) (driver.Stmt, error) {
	return &linkedinMockSQLStmt{query: query, state: c.state}, nil
}
func (c *linkedinMockSQLConn) Close() error              { return nil }
func (c *linkedinMockSQLConn) Begin() (driver.Tx, error) { return nil, nil }

type linkedinMockSQLStmt struct {
	query string
	state *linkedinMockDBState
}

func (s *linkedinMockSQLStmt) Close() error  { return nil }
func (s *linkedinMockSQLStmt) NumInput() int { return -1 }

func (s *linkedinMockSQLStmt) Exec(args []driver.Value) (driver.Result, error) {
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

func (s *linkedinMockSQLStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.state.mu.Lock()
	defer s.state.mu.Unlock()
	q := strings.ToLower(s.query)
	if strings.Contains(q, "select value from meta") {
		key := fmt.Sprint(args[0])
		val, ok := s.state.meta[key]
		if !ok {
			return &linkedinMockRows{cols: []string{"value"}}, nil
		}
		return &linkedinMockRows{
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
		return &linkedinMockRows{
			cols: []string{"id", "job_title", "company", "salary_range", "location", "slug", "salary_currency", "company_icon_image_id", "external_id", "salary_period"},
			rows: out,
		}, nil
	}
	return &linkedinMockRows{}, nil
}

type linkedinMockRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (r *linkedinMockRows) Columns() []string { return r.cols }
func (r *linkedinMockRows) Close() error      { return nil }
func (r *linkedinMockRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func newLinkedinMockDB(t *testing.T, state *linkedinMockDBState) *sql.DB {
	t.Helper()
	registerLinkedinMockDriverOnce.Do(func() {
		sql.Register("linkedin_mock_db", &linkedinMockSQLDriver{})
	})
	dsn := fmt.Sprintf("dsn_%d", time.Now().UnixNano())
	linkedinMockStates.Store(dsn, state)
	t.Cleanup(func() { linkedinMockStates.Delete(dsn) })
	db, err := sql.Open("linkedin_mock_db", dsn)
	if err != nil {
		t.Fatalf("failed to open mock db: %v", err)
	}
	return db
}

func TestParseLinkedinURNFromCandidate(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"urn:li:organization:12345", "urn:li:organization:12345"},
		{"https://www.linkedin.com/company/golang-cafe/", "urn:li:organization:golang-cafe"},
		{"https://www.linkedin.com/in/johndoe?trk=public", "urn:li:person:johndoe"},
		{"golang-cafe", "urn:li:organization:golang-cafe"},
	}
	for _, tc := range cases {
		if got := parseLinkedinURNFromCandidate(tc.in); got != tc.want {
			t.Fatalf("parseLinkedinURNFromCandidate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveLinkedinAuthorURNMeFallback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-123" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get(linkedinRestliHeader) != linkedinRestliVersion {
			t.Errorf("unexpected Restli header: %s", r.Header.Get(linkedinRestliHeader))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc987"}`))
	}))
	defer ts.Close()

	cfg := config.Config{LinkedinAccessToken: "tok-123"}
	urn, err := resolveLinkedinAuthorURN(ts.Client(), ts.URL, cfg)
	if err != nil {
		t.Fatalf("resolveLinkedinAuthorURN returned error: %v", err)
	}
	if urn != "urn:li:person:abc987" {
		t.Fatalf("expected urn:li:person:abc987, got %q", urn)
	}
}

func TestRunLinkedinSchedulerEndToEnd(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []map[string]interface{}
		headers  []http.Header
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("failed to decode ugcPosts payload: %v", err)
		}
		mu.Lock()
		requests = append(requests, body)
		headers = append(headers, r.Header.Clone())
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"urn:li:share:1"}`))
	}))
	defer ts.Close()

	state := &linkedinMockDBState{
		meta: map[string]string{lastLinkedinJobIDMetaKey: "20"},
		jobs: []linkedinMockJobRow{
			{id: 21, title: "Senior Go Engineer", company: "Acme", salary: "$150k", location: "Remote", slug: "senior-go-acme"},
			{id: 22, title: "Staff Go Engineer", company: "Beta", salary: "$190k", location: "NYC", slug: "staff-go-beta"},
		},
	}
	db := newLinkedinMockDB(t, state)
	defer db.Close()

	cfg := config.Config{
		LinkedinAccessToken: "li-secret-token",
		LinkedinPageURL:     "https://www.linkedin.com/company/golang-cafe/",
		LinkedinJobsToPost:  2,
		SiteJobCategory:     "golang",
		URLProtocol:         "https://",
		SiteHost:            "golang.cafe",
	}
	svr := server.NewServer(cfg, db, nil, nil, email.Client{}, nil)
	jobRepo := job.NewRepository(db)

	runLinkedinScheduler(svr, jobRepo, ts.Client(), ts.URL, ts.URL)

	if len(requests) != 2 {
		t.Fatalf("expected 2 LinkedIn UGC shares, got %d", len(requests))
	}
	if headers[0].Get("Authorization") != "Bearer li-secret-token" {
		t.Fatalf("expected Bearer li-secret-token, got %q", headers[0].Get("Authorization"))
	}
	if headers[0].Get(linkedinRestliHeader) != linkedinRestliVersion {
		t.Fatalf("expected X-Restli-Protocol-Version 2.0.0, got %q", headers[0].Get(linkedinRestliHeader))
	}
	if requests[0]["author"] != "urn:li:organization:golang-cafe" {
		t.Fatalf("expected author urn:li:organization:golang-cafe, got %v", requests[0]["author"])
	}
	if state.meta[lastLinkedinJobIDMetaKey] != "22" {
		t.Fatalf("expected last_linkedin_job_id=22, got %q", state.meta[lastLinkedinJobIDMetaKey])
	}
}
