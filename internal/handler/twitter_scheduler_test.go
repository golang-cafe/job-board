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
)

func TestIsWeekdayScheduleGuard(t *testing.T) {
	monday := time.Date(2026, time.November, 2, 10, 0, 0, 0, time.UTC)
	friday := time.Date(2026, time.November, 6, 18, 0, 0, 0, time.UTC)
	saturday := time.Date(2026, time.November, 7, 9, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, time.November, 8, 9, 0, 0, 0, time.UTC)

	if !isWeekday(monday) || !isWeekday(friday) {
		t.Fatalf("expected Monday and Friday to be recognized as weekdays")
	}
	if isWeekday(saturday) || isWeekday(sunday) {
		t.Fatalf("expected Saturday and Sunday to be excluded from weekday schedule")
	}
}

func TestResolveDailyTweetLimit(t *testing.T) {
	if got := resolveDailyTweetLimit(0); got != 2 {
		t.Fatalf("expected default limit 2, got %d", got)
	}
	if got := resolveDailyTweetLimit(5); got != 2 {
		t.Fatalf("expected limit capped at 2 per day, got %d", got)
	}
	if got := resolveDailyTweetLimit(1); got != 1 {
		t.Fatalf("expected configured limit 1, got %d", got)
	}
}

func TestTwitterV2ClientPostTweet(t *testing.T) {
	var receivedAuth string
	var receivedText string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedText = body["text"]
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"1","text":"ok"}}`))
	}))
	defer srv.Close()

	cfg := config.Config{
		TwitterClientKey:         "ckey",
		TwitterClientSecret:      "csecret",
		TwitterAccessToken:       "atoken",
		TwitterAccessTokenSecret: "asecret",
		SiteJobCategory:          "golang",
		URLProtocol:              "https://",
		SiteHost:                 "golang.cafe",
	}
	client := newTwitterV2Client(cfg)
	client.endpoint = srv.URL

	sample := job.JobPost{
		ID:          42,
		JobTitle:    "Senior Go Engineer",
		Company:     "Acme",
		Location:    "Remote",
		SalaryRange: "$160k - $190k",
		Slug:        "senior-go-engineer-acme",
	}
	tweet := formatJobTweet(cfg, sample)
	if err := client.postTweet(tweet); err != nil {
		t.Fatalf("expected successful tweet post, got error: %v", err)
	}
	if !strings.HasPrefix(receivedAuth, "OAuth ") || !strings.Contains(receivedAuth, `oauth_consumer_key="ckey"`) {
		t.Fatalf("unexpected OAuth header: %s", receivedAuth)
	}
	if !strings.Contains(receivedText, "https://golang.cafe/job/senior-go-engineer-acme") {
		t.Fatalf("unexpected tweet payload: %s", receivedText)
	}
}
