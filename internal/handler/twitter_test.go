package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/server"
)

type fakeTwitterJobStore struct {
	values map[string]string
	jobs   []job.JobPost
}

func (f *fakeTwitterJobStore) GetValue(key string) (string, error) {
	return f.values[key], nil
}

func (f *fakeTwitterJobStore) SetValue(key, val string) error {
	f.values[key] = val
	return nil
}

func (f *fakeTwitterJobStore) GetLastNJobsFromID(max, jobID int) ([]job.JobPost, error) {
	var out []job.JobPost
	for _, j := range f.jobs {
		if j.ID > jobID && len(out) < max {
			out = append(out, j)
		}
	}
	return out, nil
}

func TestRunTwitterJobsSchedulerWeekdayAndWeekend(t *testing.T) {
	var receivedTweets []string
	var receivedAuth []string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		receivedAuth = append(receivedAuth, r.Header.Get("Authorization"))
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		receivedTweets = append(receivedTweets, body["text"])
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"1","text":"ok"}}`))
	}))
	defer ts.Close()

	store := &fakeTwitterJobStore{
		values: map[string]string{"last_twitted_job_id": "10"},
		jobs: []job.JobPost{
			{ID: 11, JobTitle: "Senior Go Engineer", Company: "Acme", Location: "Remote", SalaryRange: "$150k", Slug: "senior-go-acme"},
			{ID: 12, JobTitle: "Backend Go Dev", Company: "Beta", Location: "NYC", SalaryRange: "$140k", Slug: "backend-go-beta"},
			{ID: 13, JobTitle: "Platform Engineer", Company: "Gamma", Location: "London", SalaryRange: "$160k", Slug: "platform-gamma"},
		},
	}

	svr := server.Server{}
	_ = config.Config{}
	client := newTwitterV2Client("ckey", "csecret", "atoken", "asecret")
	client.endpoint = ts.URL

	// 1. Weekend (Saturday) should skip posting
	saturday := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	runTwitterJobsScheduler(svr, store, client, saturday)
	if len(receivedTweets) != 0 {
		t.Fatalf("expected 0 tweets on Saturday, got %d", len(receivedTweets))
	}

	// 2. Weekday (Monday) should post at most 2 jobs and update last_twitted_job_id to 12
	monday := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	runTwitterJobsScheduler(svr, store, client, monday)
	if len(receivedTweets) != 2 {
		t.Fatalf("expected 2 tweets on Monday, got %d", len(receivedTweets))
	}
	if store.values["last_twitted_job_id"] != "12" {
		t.Fatalf("expected last_twitted_job_id=12, got %s", store.values["last_twitted_job_id"])
	}
	for _, auth := range receivedAuth {
		if !strings.HasPrefix(auth, "OAuth ") || !strings.Contains(auth, "oauth_signature=") {
			t.Fatalf("expected OAuth 1.0a header, got %s", auth)
		}
	}
}
