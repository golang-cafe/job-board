package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-cafe/job-board/internal/config"
)

func TestIsWeekday(t *testing.T) {
	monday := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	friday := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	saturday := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

	if !IsWeekday(monday) {
		t.Error("expected Monday to be a weekday")
	}
	if !IsWeekday(friday) {
		t.Error("expected Friday to be a weekday")
	}
	if IsWeekday(saturday) {
		t.Error("expected Saturday not to be a weekday")
	}
	if IsWeekday(sunday) {
		t.Error("expected Sunday not to be a weekday")
	}
}

func TestPostTweetV2(t *testing.T) {
	var authHeader string
	var body twitterV2CreateTweetRequest

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"123","text":"hello"}}`))
	}))
	defer ts.Close()

	origEndpoint := twitterV2TweetsEndpoint
	twitterV2TweetsEndpoint = ts.URL
	defer func() { twitterV2TweetsEndpoint = origEndpoint }()

	cfg := config.Config{
		TwitterClientKey:         "consumer-key",
		TwitterClientSecret:      "consumer-secret",
		TwitterAccessToken:       "access-token",
		TwitterAccessTokenSecret: "access-token-secret",
		TwitterJobsToPost:        2,
	}

	expectedText := "Go Engineer with Example - Remote | $140k - $170k\n\n#golang #golangjobs\n\nhttps://golang.cafe/job/go-engineer"
	if err := PostTweetV2(ts.Client(), cfg, expectedText); err != nil {
		t.Fatalf("PostTweetV2 returned unexpected error: %v", err)
	}

	if !strings.HasPrefix(authHeader, "OAuth ") {
		t.Errorf("expected OAuth 1.0a header, got %s", authHeader)
	}
	if !strings.Contains(authHeader, `oauth_consumer_key="consumer-key"`) {
		t.Errorf("expected oauth_consumer_key in Authorization header, got %s", authHeader)
	}
	if body.Text != expectedText {
		t.Errorf("expected tweet text %q, got %q", expectedText, body.Text)
	}
}
