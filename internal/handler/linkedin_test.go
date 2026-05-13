package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostLinkedInPost(t *testing.T) {
	var gotAuth string
	var gotVersion string
	var gotProtocol string
	var gotPayload linkedinPostRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		gotVersion = r.Header.Get("LinkedIn-Version")
		gotProtocol = r.Header.Get("X-Restli-Protocol-Version")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	err := postLinkedInPost(context.Background(), srv.Client(), srv.URL, "token", "202511", "urn:li:organization:123", "job text")
	if err != nil {
		t.Fatalf("postLinkedInPost returned error: %v", err)
	}

	if gotAuth != "Bearer token" {
		t.Fatalf("Authorization header = %q", gotAuth)
	}
	if gotVersion != "202511" {
		t.Fatalf("LinkedIn-Version header = %q", gotVersion)
	}
	if gotProtocol != "2.0.0" {
		t.Fatalf("X-Restli-Protocol-Version header = %q", gotProtocol)
	}
	if gotPayload.Author != "urn:li:organization:123" {
		t.Fatalf("author = %q", gotPayload.Author)
	}
	if gotPayload.Commentary != "job text" {
		t.Fatalf("commentary = %q", gotPayload.Commentary)
	}
	if gotPayload.Visibility != "PUBLIC" || gotPayload.LifecycleState != "PUBLISHED" {
		t.Fatalf("unexpected post state: visibility=%q lifecycle=%q", gotPayload.Visibility, gotPayload.LifecycleState)
	}
	if gotPayload.Distribution.FeedDistribution != "MAIN_FEED" {
		t.Fatalf("feed distribution = %q", gotPayload.Distribution.FeedDistribution)
	}
}

func TestPostLinkedInPostReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := postLinkedInPost(context.Background(), srv.Client(), srv.URL, "bad", "202511", "urn:li:organization:123", "job text")
	if err == nil {
		t.Fatal("postLinkedInPost returned nil error for non-2xx response")
	}
}
