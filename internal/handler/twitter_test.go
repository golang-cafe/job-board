package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
)

type fakeTwitterJobRepo struct {
	values      map[string]string
	jobs        []*job.JobPost
	getValueErr error
	setValueErr error
	getJobsErr  error
	requestedN  int
	requestedID int
}

func (f *fakeTwitterJobRepo) GetValue(key string) (string, error) {
	if f.getValueErr != nil {
		return "", f.getValueErr
	}
	return f.values[key], nil
}

func (f *fakeTwitterJobRepo) SetValue(key, val string) error {
	if f.setValueErr != nil {
		return f.setValueErr
	}
	f.values[key] = val
	return nil
}

func (f *fakeTwitterJobRepo) GetLastNJobsFromID(max, jobID int) ([]*job.JobPost, error) {
	f.requestedN = max
	f.requestedID = jobID
	if f.getJobsErr != nil {
		return nil, f.getJobsErr
	}
	return f.jobs, nil
}

func TestIsWeekday(t *testing.T) {
	cases := []struct {
		day  time.Time
		want bool
	}{
		{day: time.Date(2026, time.October, 26, 9, 0, 0, 0, time.UTC), want: true},  // Monday
		{day: time.Date(2026, time.October, 27, 9, 0, 0, 0, time.UTC), want: true},  // Tuesday
		{day: time.Date(2026, time.October, 28, 9, 0, 0, 0, time.UTC), want: true},  // Wednesday
		{day: time.Date(2026, time.October, 29, 9, 0, 0, 0, time.UTC), want: true},  // Thursday
		{day: time.Date(2026, time.October, 30, 9, 0, 0, 0, time.UTC), want: true},  // Friday
		{day: time.Date(2026, time.October, 31, 9, 0, 0, 0, time.UTC), want: false}, // Saturday
		{day: time.Date(2026, time.November, 1, 9, 0, 0, 0, time.UTC), want: false}, // Sunday
	}
	for _, tc := range cases {
		if got := isWeekday(tc.day); got != tc.want {
			t.Fatalf("isWeekday(%s) = %v, want %v", tc.day.Weekday(), got, tc.want)
		}
	}
}

func TestFormatJobTweet(t *testing.T) {
	cfg := config.Config{
		SiteJobCategory: "Golang",
		URLProtocol:     "https://",
		SiteHost:        "golang.cafe",
	}
	jobPost := &job.JobPost{
		ID:          42,
		JobTitle:    "Senior Go Engineer",
		Company:     "Acme Corp",
		Location:    "Remote",
		SalaryRange: "$150k - $180k",
		Slug:        "senior-go-engineer-acme-corp-123",
	}
	got := formatJobTweet(jobPost, cfg)
	want := "Senior Go Engineer with Acme Corp - Remote | $150k - $180k\n\n#Golang #Golangjobs\n\nhttps://golang.cafe/job/senior-go-engineer-acme-corp-123"
	if got != want {
		t.Fatalf("formatJobTweet() = %q, want %q", got, want)
	}
}

func TestRunTwitterJobsSchedulerWeekdayAndWeekendFiltering(t *testing.T) {
	cfg := config.Config{
		TwitterJobsToPost: 0, // should default to 2 jobs per day
		SiteJobCategory:   "Golang",
		URLProtocol:       "https://",
		SiteHost:          "golang.cafe",
	}
	repo := &fakeTwitterJobRepo{
		values: map[string]string{lastTwittedJobIDKey: "100"},
		jobs: []*job.JobPost{
			{
				ID:          101,
				JobTitle:    "Backend Go Developer",
				Company:     "Beta",
				Location:    "Berlin",
				SalaryRange: "€90k - €110k",
				Slug:        "backend-go-developer-beta",
			},
			{
				ID:          102,
				JobTitle:    "Platform Engineer",
				Company:     "Gamma",
				Location:    "Remote",
				SalaryRange: "$140k - $170k",
				Slug:        "platform-engineer-gamma",
			},
		},
	}

	var posted []string
	poster := func(text string) error {
		posted = append(posted, text)
		return nil
	}
	logErr := func(err error, msg string) {
		t.Fatalf("unexpected error logged (%s): %v", msg, err)
	}

	// Weekend: Saturday should skip without calling repo or poster
	saturday := time.Date(2026, time.October, 31, 10, 0, 0, 0, time.UTC)
	runTwitterJobsScheduler(cfg, repo, poster, logErr, saturday)
	if len(posted) != 0 {
		t.Fatalf("expected 0 tweets on Saturday, got %d", len(posted))
	}
	if repo.values[lastTwittedJobIDKey] != "100" {
		t.Fatalf("expected cursor to remain 100 on Saturday, got %s", repo.values[lastTwittedJobIDKey])
	}

	// Weekday: Monday should post 2 jobs and advance cursor to 102
	monday := time.Date(2026, time.October, 26, 10, 0, 0, 0, time.UTC)
	runTwitterJobsScheduler(cfg, repo, poster, logErr, monday)
	if repo.requestedN != defaultTwitterJobsPerDay {
		t.Fatalf("expected default %d jobs per day, requested %d", defaultTwitterJobsPerDay, repo.requestedN)
	}
	if len(posted) != 2 {
		t.Fatalf("expected 2 tweets on Monday, got %d", len(posted))
	}
	if repo.values[lastTwittedJobIDKey] != "102" {
		t.Fatalf("expected cursor to update to 102, got %s", repo.values[lastTwittedJobIDKey])
	}
}

func TestTwitterV2ClientPostTweetOAuth1AndPayload(t *testing.T) {
	var receivedAuth string
	var receivedContentType string
	var receivedBody twitterCreateTweetRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		receivedAuth = r.Header.Get("Authorization")
		receivedContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("unable to decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"1900000000000000001","text":"hello world"}}`))
	}))
	defer srv.Close()

	client := &twitterV2Client{
		endpoint:          srv.URL,
		httpClient:        srv.Client(),
		clientKey:         "consumer-key-123",
		clientSecret:      "consumer-secret-456",
		accessToken:       "access-token-789",
		accessTokenSecret: "access-secret-012",
		nowFn: func() time.Time {
			return time.Unix(1700000000, 0).UTC()
		},
		nonceFn: func() (string, error) {
			return "fixednonce123456", nil
		},
	}

	tweetText := "Go Engineer with Example - Remote | $120k - $150k\n\n#Golang #Golangjobs\n\nhttps://golang.cafe/job/go-engineer"
	if err := client.postTweet(tweetText); err != nil {
		t.Fatalf("postTweet returned unexpected error: %v", err)
	}
	if receivedContentType != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", receivedContentType)
	}
	if receivedBody.Text != tweetText {
		t.Fatalf("expected tweet text %q, got %q", tweetText, receivedBody.Text)
	}
	if !strings.HasPrefix(receivedAuth, "OAuth ") {
		t.Fatalf("expected OAuth 1.0a Authorization header, got %q", receivedAuth)
	}
	for _, expectedFragment := range []string{
		`oauth_consumer_key="consumer-key-123"`,
		`oauth_nonce="fixednonce123456"`,
		`oauth_signature_method="HMAC-SHA1"`,
		`oauth_timestamp="1700000000"`,
		`oauth_token="access-token-789"`,
		`oauth_version="1.0"`,
		`oauth_signature="`,
	} {
		if !strings.Contains(receivedAuth, expectedFragment) {
			t.Fatalf("expected Authorization header %q to contain %s", receivedAuth, expectedFragment)
		}
	}
}

func TestTwitterV2ClientErrorHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"title":"Forbidden","detail":"You are not permitted to perform this action."}`))
	}))
	defer srv.Close()

	client := &twitterV2Client{
		endpoint:    srv.URL,
		httpClient:  srv.Client(),
		accessToken: "bearer-token-only",
		nowFn:       time.Now,
		nonceFn:     func() (string, error) { return "nonce", nil },
	}
	err := client.postTweet("test tweet")
	if err == nil || !strings.Contains(err.Error(), "twitter api returned status 403") {
		t.Fatalf("expected 403 error, got %v", err)
	}

	repo := &fakeTwitterJobRepo{
		values: map[string]string{lastTwittedJobIDKey: "50"},
		jobs: []*job.JobPost{
			{ID: 51, JobTitle: "Role 1", Company: "Co 1", Location: "Remote", SalaryRange: "$100k", Slug: "role-1"},
		},
	}
	var loggedErrors int
	runTwitterJobsScheduler(
		config.Config{TwitterJobsToPost: 2},
		repo,
		func(string) error { return errors.New("post failed") },
		func(error, string) { loggedErrors++ },
		time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC),
	)
	if loggedErrors != 1 {
		t.Fatalf("expected 1 logged error, got %d", loggedErrors)
	}
	if repo.values[lastTwittedJobIDKey] != "50" {
		t.Fatalf("expected cursor to stay at 50 when all tweets fail, got %s", repo.values[lastTwittedJobIDKey])
	}
}
