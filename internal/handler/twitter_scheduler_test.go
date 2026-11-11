package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-cafe/job-board/internal/job"
)

func TestIsTwitterPostingWeekday(t *testing.T) {
	monday := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	friday := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	saturday := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

	if !IsTwitterPostingWeekday(monday) {
		t.Fatalf("expected Monday to be a valid Twitter posting weekday")
	}
	if !IsTwitterPostingWeekday(friday) {
		t.Fatalf("expected Friday to be a valid Twitter posting weekday")
	}
	if IsTwitterPostingWeekday(saturday) {
		t.Fatalf("expected Saturday to be skipped")
	}
	if IsTwitterPostingWeekday(sunday) {
		t.Fatalf("expected Sunday to be skipped")
	}
}

func TestEffectiveDailyTwitterJobs(t *testing.T) {
	if got := EffectiveDailyTwitterJobs(0); got != 2 {
		t.Fatalf("expected default 2 jobs/day, got %d", got)
	}
	if got := EffectiveDailyTwitterJobs(5); got != 2 {
		t.Fatalf("expected cap of 2 jobs/day, got %d", got)
	}
	if got := EffectiveDailyTwitterJobs(1); got != 1 {
		t.Fatalf("expected 1 job/day when configured=1, got %d", got)
	}
}

func TestPostTweetV2(t *testing.T) {
	var receivedAuth string
	var receivedText string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		receivedAuth = r.Header.Get("Authorization")
		var payload twitterV2TweetRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("unable to decode request payload: %v", err)
		}
		receivedText = payload.Text
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"12345","text":"ok"}}`))
	}))
	defer ts.Close()

	sampleJob := job.Post{
		ID:          42,
		JobTitle:    "Senior Go Engineer",
		Company:     "Acme Corp",
		Location:    "Remote",
		SalaryRange: "$160k - $190k",
		Slug:        "senior-go-engineer-acme-corp",
	}
	tweetText := FormatJobTweet(sampleJob, "golang", "https://", "golang.cafe")
	err := PostTweetV2(ts.Client(), ts.URL, "ckey", "csecret", "atoken", "asecret", tweetText)
	if err != nil {
		t.Fatalf("expected PostTweetV2 to succeed, got error: %v", err)
	}
	if !strings.HasPrefix(receivedAuth, "OAuth ") || !strings.Contains(receivedAuth, "oauth_signature=") {
		t.Fatalf("expected OAuth 1.0a Authorization header, got: %s", receivedAuth)
	}
	if !strings.Contains(receivedText, "Senior Go Engineer with Acme Corp") {
		t.Fatalf("unexpected tweet text: %s", receivedText)
	}
}
