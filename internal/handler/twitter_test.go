package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-cafe/job-board/internal/job"
)

func TestPostTweetToXAPIUsesOAuthAndJSON(t *testing.T) {
	var called bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/2/tweets" {
			t.Fatalf("path = %s, want /2/tweets", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type = %s, want application/json", got)
		}
		auth := r.Header.Get("Authorization")
		for _, want := range []string{
			"OAuth ",
			`oauth_consumer_key="client-key"`,
			`oauth_signature_method="HMAC-SHA1"`,
			`oauth_signature=`,
			`oauth_token="access-token"`,
		} {
			if !strings.Contains(auth, want) {
				t.Fatalf("authorization header %q does not contain %q", auth, want)
			}
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["text"] != "Senior Go Engineer" {
			t.Fatalf("tweet body text = %q", body["text"])
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	err := postTweetToXAPI(context.Background(), ts.Client(), ts.URL+"/2/tweets", twitterOAuthCredentials{
		AccessToken:       "access-token",
		AccessTokenSecret: "access-secret",
		ClientKey:         "client-key",
		ClientSecret:      "client-secret",
	}, "Senior Go Engineer")
	if err != nil {
		t.Fatalf("postTweetToXAPI returned error: %v", err)
	}
	if !called {
		t.Fatal("expected test server to be called")
	}
}

func TestPostTweetToXAPIReturnsAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"bad token"}`))
	}))
	defer ts.Close()

	err := postTweetToXAPI(context.Background(), ts.Client(), ts.URL, twitterOAuthCredentials{
		AccessToken:       "access-token",
		AccessTokenSecret: "access-secret",
		ClientKey:         "client-key",
		ClientSecret:      "client-secret",
	}, "Senior Go Engineer")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "bad token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatJobTweetLimitsTextAndKeepsJobURL(t *testing.T) {
	tweet := formatJobTweet(job.JobPost{
		JobTitle:    strings.Repeat("Senior Go Engineer ", 20),
		Company:     "Golang Cafe",
		Location:    "Remote",
		SalaryRange: "$100k-$150k",
		Slug:        "senior-go-engineer",
	}, "Go", "https://", "golang.cafe")

	if got := len([]rune(tweet)); got > twitterMaxTweetRunes {
		t.Fatalf("tweet has %d runes, want <= %d", got, twitterMaxTweetRunes)
	}
	for _, want := range []string{"https://golang.cafe/job/senior-go-engineer", "#Go #Gojobs"} {
		if !strings.Contains(tweet, want) {
			t.Fatalf("tweet %q does not contain %q", tweet, want)
		}
	}
}

func TestShouldRunTwitterJobSchedulerWeekdaysOnly(t *testing.T) {
	if !shouldRunTwitterJobScheduler(time.Date(2026, 5, 20, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("expected Wednesday to run")
	}
	if shouldRunTwitterJobScheduler(time.Date(2026, 5, 23, 9, 0, 0, 0, time.UTC)) {
		t.Fatal("expected Saturday to be skipped")
	}
}
