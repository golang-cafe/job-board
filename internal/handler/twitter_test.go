package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostTwitterTweet(t *testing.T) {
	var gotAuth string
	var gotPayload twitterCreateTweetRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	err := postTwitterTweet(context.Background(), srv.Client(), srv.URL, "token", "job text")
	if err != nil {
		t.Fatalf("postTwitterTweet returned error: %v", err)
	}

	if gotAuth != "Bearer token" {
		t.Fatalf("Authorization header = %q", gotAuth)
	}
	if gotPayload.Text != "job text" {
		t.Fatalf("tweet text = %q", gotPayload.Text)
	}
}

func TestPostTwitterTweetReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := postTwitterTweet(context.Background(), srv.Client(), srv.URL, "bad", "job text")
	if err == nil {
		t.Fatal("postTwitterTweet returned nil error for non-2xx response")
	}
}
