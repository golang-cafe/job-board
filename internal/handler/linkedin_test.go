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

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/email"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/gorilla/mux"
)

type mockLinkedInDBState struct {
	mu   sync.Mutex
	meta map[string]string
	jobs []*job.JobPost
}

var (
	registerLinkedInMockOnce sync.Once
	activeLinkedInState      *mockLinkedInDBState
	activeLinkedInStateMu    sync.Mutex
)

func setLinkedInMockDB(st *mockLinkedInDBState) *sql.DB {
	registerLinkedInMockOnce.Do(func() {
		sql.Register("mock_linkedin_job_board_db", &mockLinkedInSQLDriver{})
	})
	activeLinkedInStateMu.Lock()
	activeLinkedInState = st
	activeLinkedInStateMu.Unlock()
	db, err := sql.Open("mock_linkedin_job_board_db", "test")
	if err != nil {
		panic(err)
	}
	return db
}

type mockLinkedInSQLDriver struct{}

func (d *mockLinkedInSQLDriver) Open(name string) (driver.Conn, error) {
	return &mockLinkedInSQLConn{}, nil
}

type mockLinkedInSQLConn struct{}

func (c *mockLinkedInSQLConn) Prepare(query string) (driver.Stmt, error) {
	return &mockLinkedInSQLStmt{query: query}, nil
}

func (c *mockLinkedInSQLConn) Close() error              { return nil }
func (c *mockLinkedInSQLConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("tx not supported") }

type mockLinkedInSQLStmt struct {
	query string
}

func (s *mockLinkedInSQLStmt) Close() error  { return nil }
func (s *mockLinkedInSQLStmt) NumInput() int { return -1 }

func (s *mockLinkedInSQLStmt) Exec(args []driver.Value) (driver.Result, error) {
	activeLinkedInStateMu.Lock()
	st := activeLinkedInState
	activeLinkedInStateMu.Unlock()
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

func (s *mockLinkedInSQLStmt) Query(args []driver.Value) (driver.Rows, error) {
	activeLinkedInStateMu.Lock()
	st := activeLinkedInState
	activeLinkedInStateMu.Unlock()
	st.mu.Lock()
	defer st.mu.Unlock()

	q := strings.ToLower(strings.TrimSpace(s.query))
	switch {
	case strings.HasPrefix(q, "select value from meta"):
		key := fmt.Sprint(args[0])
		val, ok := st.meta[key]
		if !ok {
			return &mockLinkedInSQLRows{cols: []string{"value"}, rows: nil}, nil
		}
		return &mockLinkedInSQLRows{
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
		return &mockLinkedInSQLRows{
			cols: []string{"id", "job_title", "company", "salary_range", "location", "slug", "salary_currency", "company_icon_image_id", "external_id", "salary_period"},
			rows: matched,
		}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", s.query)
	}
}

type mockLinkedInSQLRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (r *mockLinkedInSQLRows) Columns() []string { return r.cols }
func (r *mockLinkedInSQLRows) Close() error      { return nil }
func (r *mockLinkedInSQLRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

func TestNormalizeLinkedInAuthorURN(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "already organization URN",
			in:   "urn:li:organization:987654",
			want: "urn:li:organization:987654",
		},
		{
			name: "already person URN",
			in:   "urn:li:person:abc123XYZ",
			want: "urn:li:person:abc123XYZ",
		},
		{
			name: "full company page URL with trailing slash",
			in:   "https://www.linkedin.com/company/golang-cafe/",
			want: "urn:li:organization:golang-cafe",
		},
		{
			name: "personal profile URL",
			in:   "https://www.linkedin.com/in/johndoe/",
			want: "urn:li:person:johndoe",
		},
		{
			name: "plain organization id or slug",
			in:   "golangcafe",
			want: "urn:li:organization:golangcafe",
		},
		{
			name: "empty input",
			in:   "   ",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeLinkedInAuthorURN(tc.in); got != tc.want {
				t.Fatalf("normalizeLinkedInAuthorURN(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPostLinkedInUGCPost(t *testing.T) {
	sampleJob := &job.JobPost{
		ID:          21,
		JobTitle:    "Senior Go Engineer",
		Company:     "Uber",
		Location:    "Amsterdam, Netherlands",
		SalaryRange: "€110k - €140k",
		Slug:        "senior-go-engineer-uber",
	}

	tests := []struct {
		name           string
		cfg            config.Config
		serverStatus   int
		serverResponse string
		wantAuthorURN  string
		wantErr        bool
	}{
		{
			name: "valid UGC post request succeeds on 201 Created",
			cfg: config.Config{
				LinkedInAPIToken: "li_token_secret_123",
				LinkedInPageURL:  "https://www.linkedin.com/company/golang-cafe/",
				SiteJobCategory:  "golang",
				URLProtocol:      "https://",
				SiteHost:         "golang.cafe",
			},
			serverStatus:   http.StatusCreated,
			serverResponse: `{"id":"urn:li:share:7000000000000000001"}`,
			wantAuthorURN:  "urn:li:organization:golang-cafe",
			wantErr:        false,
		},
		{
			name: "falls back to SiteLinkedin when LinkedInPageURL is empty",
			cfg: config.Config{
				LinkedInAPIToken: "li_token_secret_123",
				SiteLinkedin:     "urn:li:organization:1234567",
				SiteJobCategory:  "golang",
				URLProtocol:      "https://",
				SiteHost:         "golang.cafe",
			},
			serverStatus:   http.StatusCreated,
			serverResponse: `{"id":"urn:li:share:7000000000000000002"}`,
			wantAuthorURN:  "urn:li:organization:1234567",
			wantErr:        false,
		},
		{
			name: "non-2xx status from LinkedIn API returns error",
			cfg: config.Config{
				LinkedInAPIToken: "li_token_secret_123",
				LinkedInPageURL:  "urn:li:organization:1234567",
				SiteJobCategory:  "golang",
				URLProtocol:      "https://",
				SiteHost:         "golang.cafe",
			},
			serverStatus:   http.StatusForbidden,
			serverResponse: `{"message":"Access to posting shares denied","status":403}`,
			wantErr:        true,
		},
		{
			name: "missing LinkedIn token returns error before request",
			cfg: config.Config{
				LinkedInPageURL: "urn:li:organization:1234567",
			},
			serverStatus: http.StatusCreated,
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotAuth, gotRestli, gotContentType string
			var gotPayload linkedInUGCPostRequest

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", r.Method)
				}
				if r.URL.Path != "/v2/ugcPosts" {
					t.Errorf("path = %s, want /v2/ugcPosts", r.URL.Path)
				}
				gotAuth = r.Header.Get("Authorization")
				gotRestli = r.Header.Get("X-Restli-Protocol-Version")
				gotContentType = r.Header.Get("Content-Type")
				if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
					t.Errorf("unable to decode LinkedIn UGC payload: %v", err)
				}
				w.Header().Set("X-RestLi-Id", "urn:li:share:7000000000000000001")
				w.WriteHeader(tc.serverStatus)
				_, _ = w.Write([]byte(tc.serverResponse))
			}))
			defer srv.Close()

			err := postLinkedInUGCPost(srv.Client(), srv.URL+"/v2/ugcPosts", tc.cfg, sampleJob)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotAuth != "Bearer "+tc.cfg.LinkedInAPIToken {
				t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer "+tc.cfg.LinkedInAPIToken)
			}
			if gotRestli != "2.0.0" {
				t.Errorf("X-Restli-Protocol-Version = %q, want 2.0.0", gotRestli)
			}
			if gotContentType != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", gotContentType)
			}
			if gotPayload.Author != tc.wantAuthorURN {
				t.Errorf("Author = %q, want %q", gotPayload.Author, tc.wantAuthorURN)
			}
			if gotPayload.LifecycleState != "PUBLISHED" {
				t.Errorf("LifecycleState = %q, want PUBLISHED", gotPayload.LifecycleState)
			}
			if gotPayload.Visibility.MemberNetworkVisibility != "PUBLIC" {
				t.Errorf("Visibility = %q, want PUBLIC", gotPayload.Visibility.MemberNetworkVisibility)
			}
			shareContent := gotPayload.SpecificContent.ShareContent
			if !strings.Contains(shareContent.ShareCommentary.Text, "Senior Go Engineer with Uber") {
				t.Errorf("ShareCommentary.Text = %q, missing job headline", shareContent.ShareCommentary.Text)
			}
			if shareContent.ShareMediaCategory != "ARTICLE" || len(shareContent.Media) != 1 {
				t.Fatalf("ShareMediaCategory=%q len(Media)=%d, want ARTICLE and 1 media entry", shareContent.ShareMediaCategory, len(shareContent.Media))
			}
			if shareContent.Media[0].OriginalURL != "https://golang.cafe/job/senior-go-engineer-uber" {
				t.Errorf("Media[0].OriginalURL = %q, want job URL", shareContent.Media[0].OriginalURL)
			}
		})
	}
}

func TestRunLinkedInSchedulerAndHandler(t *testing.T) {
	sampleJobs := []*job.JobPost{
		{
			ID:          31,
			JobTitle:    "Distributed Systems Engineer",
			Company:     "Cockroach Labs",
			Location:    "Remote",
			SalaryRange: "$160k - $200k",
			Slug:        "distributed-systems-engineer-cockroach-labs",
		},
		{
			ID:          32,
			JobTitle:    "Backend Go Developer",
			Company:     "Deliveroo",
			Location:    "London, UK",
			SalaryRange: "£95k - £120k",
			Slug:        "backend-go-developer-deliveroo",
		},
		{
			ID:          33,
			JobTitle:    "Infra Engineer",
			Company:     "HashiCorp",
			Location:    "Remote",
			SalaryRange: "$150k - $185k",
			Slug:        "infra-engineer-hashicorp",
		},
	}

	t.Run("posts configured number of jobs to LinkedIn and advances last_linkedin_job_id", func(t *testing.T) {
		var postedPayloads []linkedInUGCPostRequest
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var p linkedInUGCPostRequest
			_ = json.NewDecoder(r.Body).Decode(&p)
			postedPayloads = append(postedPayloads, p)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"urn:li:share:1"}`))
		}))
		defer srv.Close()

		st := &mockLinkedInDBState{
			meta: map[string]string{}, // empty meta to verify initial default "0" and upsert
			jobs: sampleJobs,
		}
		db := setLinkedInMockDB(st)
		defer db.Close()

		cfg := config.Config{
			MachineToken:       "secret-machine-token",
			TwitterJobsToPost:  2,
			LinkedInJobsToPost: 2,
			LinkedInAPIToken:   "li_token_abc",
			LinkedInPageURL:    "https://www.linkedin.com/company/golang-cafe",
			SiteJobCategory:    "golang",
			URLProtocol:        "https://",
			SiteHost:           "golang.cafe",
		}
		svr := server.NewServer(cfg, db, mux.NewRouter(), nil, email.Client{}, nil)
		jobRepo := job.NewRepository(db)

		runLinkedInScheduler(svr, jobRepo, srv.Client(), srv.URL+"/v2/ugcPosts")

		if len(postedPayloads) != 2 {
			t.Fatalf("expected 2 LinkedIn UGC posts, got %d", len(postedPayloads))
		}
		if got := st.meta["last_linkedin_job_id"]; got != "32" {
			t.Fatalf("last_linkedin_job_id = %q, want 32", got)
		}
	})

	t.Run("TriggerLinkedInShareJobs HTTP handler enforces machine token auth", func(t *testing.T) {
		st := &mockLinkedInDBState{
			meta: map[string]string{"last_linkedin_job_id": "33"},
			jobs: nil,
		}
		db := setLinkedInMockDB(st)
		defer db.Close()

		cfg := config.Config{
			MachineToken:       "secret-machine-token",
			LinkedInJobsToPost: 2,
		}
		svr := server.NewServer(cfg, db, mux.NewRouter(), nil, email.Client{}, nil)
		jobRepo := job.NewRepository(db)
		h := TriggerLinkedInShareJobs(svr, jobRepo)

		reqUnauth := httptest.NewRequest(http.MethodPost, "/x/task/linkedin-share-jobs", nil)
		recUnauth := httptest.NewRecorder()
		h.ServeHTTP(recUnauth, reqUnauth)
		if recUnauth.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized status = %d, want %d", recUnauth.Code, http.StatusUnauthorized)
		}

		reqAuth := httptest.NewRequest(http.MethodPost, "/x/task/linkedin-share-jobs", nil)
		reqAuth.Header.Set("x-machine-token", "secret-machine-token")
		recAuth := httptest.NewRecorder()
		h.ServeHTTP(recAuth, reqAuth)
		if recAuth.Code != http.StatusOK {
			t.Fatalf("authorized status = %d, want %d", recAuth.Code, http.StatusOK)
		}
	})
}
