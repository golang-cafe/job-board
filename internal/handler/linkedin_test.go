package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/server"
)

type mockLinkedinJobStore struct {
	values map[string]string
	jobs   []job.JobPost
}

func (m *mockLinkedinJobStore) GetValue(key string) (string, error) {
	return m.values[key], nil
}

func (m *mockLinkedinJobStore) SetValue(key, val string) error {
	m.values[key] = val
	return nil
}

func (m *mockLinkedinJobStore) GetLastNJobsFromID(max, jobID int) ([]job.JobPost, error) {
	res := make([]job.JobPost, 0, len(m.jobs))
	for _, j := range m.jobs {
		if j.ID > jobID && len(res) < max {
			res = append(res, j)
		}
	}
	return res, nil
}

func TestLinkedinJobsSchedulerSharesPostsAndUpdatesCursor(t *testing.T) {
	var receivedRequests []linkedinUGCPostRequest
	var receivedHeaders []http.Header

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		receivedHeaders = append(receivedHeaders, r.Header.Clone())
		var payload linkedinUGCPostRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		receivedRequests = append(receivedRequests, payload)
		w.WriteHeader(http.StatusCreated)
		if _, writeErr := w.Write([]byte(`{"id":"urn:li:share:12345"}`)); writeErr != nil {
			t.Fatalf("failed to write response: %v", writeErr)
		}
	}))
	defer ts.Close()

	store := &mockLinkedinJobStore{
		values: map[string]string{lastLinkedinJobIDMetaKey: "10"},
		jobs: []job.JobPost{
			{
				ID:          11,
				JobTitle:    "Senior Go Engineer",
				Company:     "Acme Corp",
				Location:    "Remote",
				SalaryRange: "$140k - $180k",
				Slug:        "senior-go-engineer-acme-11",
			},
			{
				ID:          12,
				JobTitle:    "Distributed Systems Engineer",
				Company:     "Beta Labs",
				Location:    "New York",
				SalaryRange: "$160k - $200k",
				Slug:        "distributed-systems-engineer-beta-12",
			},
		},
	}

	cfg := config.Config{
		SiteJobCategory:    "golang",
		URLProtocol:        "https://",
		SiteHost:           "golang.cafe",
		LinkedinJobsToPost: 2,
	}
	svr := server.NewServer(cfg, nil, nil, nil, nil, nil)

	client := newLinkedinClient("test-linkedin-token", "", "https://www.linkedin.com/company/golang-cafe")
	client.ugcEndpoint = ts.URL

	runLinkedinJobsScheduler(svr, store, client)

	if len(receivedRequests) != 2 {
		t.Fatalf("expected 2 LinkedIn UGC post requests, got %d", len(receivedRequests))
	}
	if got := receivedHeaders[0].Get("Authorization"); got != "Bearer test-linkedin-token" {
		t.Fatalf("unexpected Authorization header: %s", got)
	}
	if got := receivedHeaders[0].Get(linkedinRestliHeader); got != linkedinRestliVersion {
		t.Fatalf("unexpected %s header: %s", linkedinRestliHeader, got)
	}
	if receivedRequests[0].Author != "urn:li:organization:golang-cafe" {
		t.Fatalf("unexpected author URN: %s", receivedRequests[0].Author)
	}
	shareContent := receivedRequests[0].SpecificContent["com.linkedin.ugc.ShareContent"]
	if !strings.Contains(shareContent.ShareCommentary.Text, "Senior Go Engineer with Acme Corp") {
		t.Fatalf("unexpected share commentary: %s", shareContent.ShareCommentary.Text)
	}
	if len(shareContent.Media) != 1 || shareContent.Media[0].OriginalURL != "https://golang.cafe/job/senior-go-engineer-acme-11" {
		t.Fatalf("unexpected media URL: %+v", shareContent.Media)
	}
	if store.values[lastLinkedinJobIDMetaKey] != "12" {
		t.Fatalf("expected last_linkedin_job_id to be updated to 12, got %s", store.values[lastLinkedinJobIDMetaKey])
	}
}

func TestLinkedinClientResolvesAuthorFromMeEndpoint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/me" {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"id":"abc987"}`)); err != nil {
				t.Fatalf("failed to write response: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write([]byte(`{"id":"urn:li:share:999"}`)); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	defer ts.Close()

	client := newLinkedinClient("token-xyz", "", "")
	client.meEndpoint = ts.URL + "/v2/me"
	client.ugcEndpoint = ts.URL + "/v2/ugcPosts"

	if err := client.shareJobPost("Hello LinkedIn", "https://golang.cafe/job/test", "Test Job", "Remote"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.authorURN != "urn:li:person:abc987" {
		t.Fatalf("expected resolved author URN urn:li:person:abc987, got %s", client.authorURN)
	}
}
