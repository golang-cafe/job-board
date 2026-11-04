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
	activeLinkedinMockState        *linkedinMockDBState
)

type linkedinMockSQLDriver struct{}

func (d *linkedinMockSQLDriver) Open(name string) (driver.Conn, error) {
	return &linkedinMockSQLConn{}, nil
}

type linkedinMockSQLConn struct{}

func (c *linkedinMockSQLConn) Prepare(query string) (driver.Stmt, error) {
	return &linkedinMockSQLStmt{query: query}, nil
}
func (c *linkedinMockSQLConn) Close() error              { return nil }
func (c *linkedinMockSQLConn) Begin() (driver.Tx, error) { return &linkedinMockSQLTx{}, nil }

type linkedinMockSQLTx struct{}

func (t *linkedinMockSQLTx) Commit() error   { return nil }
func (t *linkedinMockSQLTx) Rollback() error { return nil }

type linkedinMockSQLStmt struct {
	query string
}

func (s *linkedinMockSQLStmt) Close() error  { return nil }
func (s *linkedinMockSQLStmt) NumInput() int { return -1 }

func (s *linkedinMockSQLStmt) Exec(args []driver.Value) (driver.Result, error) {
	activeLinkedinMockState.mu.Lock()
	defer activeLinkedinMockState.mu.Unlock()
	if strings.Contains(s.query, "INSERT INTO meta") && len(args) >= 2 {
		k, _ := args[0].(string)
		v, _ := args[1].(string)
		activeLinkedinMockState.meta[k] = v
	} else if strings.Contains(s.query, "UPDATE meta") && len(args) >= 2 {
		v, _ := args[0].(string)
		k, _ := args[1].(string)
		activeLinkedinMockState.meta[k] = v
	}
	return driver.RowsAffected(1), nil
}

func (s *linkedinMockSQLStmt) Query(args []driver.Value) (driver.Rows, error) {
	activeLinkedinMockState.mu.Lock()
	defer activeLinkedinMockState.mu.Unlock()
	if strings.Contains(s.query, "SELECT value FROM meta") {
		key, _ := args[0].(string)
		val, ok := activeLinkedinMockState.meta[key]
		if !ok {
			return &linkedinMockSQLRows{cols: []string{"value"}}, nil
		}
		return &linkedinMockSQLRows{
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
		for _, j := range activeLinkedinMockState.jobs {
			if j.id > minID && int64(len(out)) < maxRows {
				out = append(out, []driver.Value{
					j.id, j.title, j.company, j.salary, j.location, j.slug, j.currency, j.iconID, j.externalID, j.period,
				})
			}
		}
		return &linkedinMockSQLRows{
			cols: []string{"id", "job_title", "company", "salary_range", "location", "slug", "salary_currency", "company_icon_image_id", "external_id", "salary_period"},
			rows: out,
		}, nil
	}
	return &linkedinMockSQLRows{}, nil
}

type linkedinMockSQLRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (r *linkedinMockSQLRows) Columns() []string { return r.cols }
func (r *linkedinMockSQLRows) Close() error      { return nil }
func (r *linkedinMockSQLRows) Next(dest []driver.Value) error {
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
	activeLinkedinMockState = state
	db, err := sql.Open("linkedin_mock_db", "mock")
	if err != nil {
		t.Fatalf("failed to open mock db: %v", err)
	}
	return db
}

func TestResolveLinkedinAuthorURN(t *testing.T) {
	if got := resolveLinkedinAuthorURN(config.Config{LinkedinAuthorURN: "urn:li:organization:999"}); got != "urn:li:organization:999" {
		t.Fatalf("expected urn:li:organization:999, got %s", got)
	}
	if got := resolveLinkedinAuthorURN(config.Config{LinkedinPageURL: "https://www.linkedin.com/company/golang-cafe/"}); got != "urn:li:organization:golang-cafe" {
		t.Fatalf("expected urn:li:organization:golang-cafe, got %s", got)
	}
	if got := resolveLinkedinAuthorURN(config.Config{LinkedinPageURL: "https://www.linkedin.com/in/johndoe"}); got != "urn:li:person:johndoe" {
		t.Fatalf("expected urn:li:person:johndoe, got %s", got)
	}
	if got := resolveLinkedinAuthorURN(config.Config{SiteLinkedin: "golang-cafe"}); got != "urn:li:organization:golang-cafe" {
		t.Fatalf("expected urn:li:organization:golang-cafe, got %s", got)
	}
}

func TestTriggerLinkedinScheduler_Unauthorized(t *testing.T) {
	cfg := config.Config{MachineToken: "secret-machine-token"}
	svr := server.NewServer(cfg, nil, nil, nil, email.Client{}, nil)
	h := TriggerLinkedinScheduler(svr, nil)

	req := httptest.NewRequest(http.MethodPost, "/x/task/linkedin-share-jobs", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 when header missing, got %d", rec.Code)
	}

	reqWrong := httptest.NewRequest(http.MethodPost, "/x/task/linkedin-share-jobs", nil)
	reqWrong.Header.Set("X-Machine-Token", "wrong-token")
	recWrong := httptest.NewRecorder()
	h.ServeHTTP(recWrong, reqWrong)
	if recWrong.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 when header wrong, got %d", recWrong.Code)
	}
}

func TestRunLinkedinScheduler_PostsUGCShareAndUpdatesCursor(t *testing.T) {
	var receivedPayloads []map[string]interface{}
	var receivedAuth []string
	var receivedRestli []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		receivedAuth = append(receivedAuth, r.Header.Get("Authorization"))
		receivedRestli = append(receivedRestli, r.Header.Get("X-Restli-Protocol-Version"))
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode linkedin payload: %v", err)
		}
		receivedPayloads = append(receivedPayloads, payload)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"urn:li:share:123456"}`))
	}))
	defer ts.Close()

	state := &linkedinMockDBState{
		meta: map[string]string{},
		jobs: []linkedinMockJobRow{
			{id: 21, title: "Senior Go Engineer", company: "Acme", salary: "$150k - $180k", location: "Remote", slug: "senior-go-engineer-acme"},
			{id: 22, title: "Distributed Systems Go Engineer", company: "Beta", salary: "$170k - $200k", location: "London", slug: "distributed-systems-go-beta"},
		},
	}
	db := newLinkedinMockDB(t, state)
	defer db.Close()

	cfg := config.Config{
		LinkedinAccessToken: "li-access-token-xyz",
		LinkedinAuthorURN:   "urn:li:organization:12345",
		LinkedinJobsToPost:  2,
		LinkedinAPIURL:      ts.URL,
		SiteJobCategory:     "golang",
		URLProtocol:         "https://",
		SiteHost:            "golang.cafe",
	}
	svr := server.NewServer(cfg, db, nil, nil, email.Client{}, nil)
	jobRepo := job.NewRepository(db)

	runLinkedinScheduler(svr, jobRepo, ts.Client(), ts.URL)

	if len(receivedPayloads) != 2 {
		t.Fatalf("expected 2 LinkedIn UGC posts, got %d", len(receivedPayloads))
	}
	if receivedAuth[0] != "Bearer li-access-token-xyz" {
		t.Fatalf("expected Bearer token header, got %s", receivedAuth[0])
	}
	if receivedRestli[0] != "2.0.0" {
		t.Fatalf("expected X-Restli-Protocol-Version 2.0.0, got %s", receivedRestli[0])
	}
	if receivedPayloads[0]["author"] != "urn:li:organization:12345" {
		t.Fatalf("expected author urn:li:organization:12345, got %v", receivedPayloads[0]["author"])
	}
	if state.meta["last_linkedin_job_id"] != "22" {
		t.Fatalf("expected last_linkedin_job_id to be updated to 22, got %s", state.meta["last_linkedin_job_id"])
	}
}
