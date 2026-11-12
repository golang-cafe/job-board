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

func TestIsWeekday(t *testing.T) {
	monday := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	saturday := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	if !isWeekday(monday) {
		t.Fatal("expected Monday to be a weekday")
	}
	if isWeekday(saturday) {
		t.Fatal("expected Saturday to not be a weekday")
	}
	if isWeekday(sunday) {
		t.Fatal("expected Sunday to not be a weekday")
	}
}

func TestPostTweetV2(t *testing.T) {
	var receivedText string
	var receivedAuth string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		receivedAuth = r.Header.Get("Authorization")
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		receivedText = body["text"]
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"123","text":"ok"}}`))
	}))
	defer ts.Close()

	sampleJob := job.JobPost{
		ID:          42,
		JobTitle:    "Senior Go Engineer",
		Company:     "Acme Corp",
		Location:    "Remote",
		SalaryRange: "$160k - $190k",
		Slug:        "senior-go-engineer-acme-42",
	}
	tweet := formatJobTweet(sampleJob, "golang", "https://", "golang.cafe")
	err := postTweetV2(ts.Client(), ts.URL, tweet, "ckey", "csecret", "atoken", "asecret")
	if err != nil {
		t.Fatalf("postTweetV2 returned error: %v", err)
	}
	if !strings.HasPrefix(receivedAuth, "OAuth ") {
		t.Fatalf("expected OAuth header, got %q", receivedAuth)
	}
	if !strings.Contains(receivedText, "Senior Go Engineer with Acme Corp") {
		t.Fatalf("unexpected tweet text: %q", receivedText)
	}
}
