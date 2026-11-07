package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
)

func TestNormalizeLinkedInAuthorURN(t *testing.T) {
	if got := normalizeLinkedInAuthorURN("urn:li:organization:12345"); got != "urn:li:organization:12345" {
		t.Fatalf("expected existing URN preserved, got %s", got)
	}
	if got := normalizeLinkedInAuthorURN("https://www.linkedin.com/company/golang-cafe/"); got != "urn:li:organization:golang-cafe" {
		t.Fatalf("expected extracted company URN, got %s", got)
	}
	if got := normalizeLinkedInAuthorURN("987654"); got != "urn:li:organization:987654" {
		t.Fatalf("expected organization URN, got %s", got)
	}
}

func TestLinkedInClientShareJobPost(t *testing.T) {
	var authHeader string
	var restliHeader string
	var decoded map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		restliHeader = r.Header.Get("X-Restli-Protocol-Version")
		_ = json.NewDecoder(r.Body).Decode(&decoded)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"urn:li:share:100"}`))
	}))
	defer srv.Close()

	cfg := config.Config{
		LinkedinAccessToken: "li-secret-token",
		LinkedinPageURL:     "https://www.linkedin.com/company/123456/",
		SiteJobCategory:     "golang",
		URLProtocol:         "https://",
		SiteHost:            "golang.cafe",
	}
	client := newLinkedInClient(cfg)
	client.endpoint = srv.URL

	sample := job.JobPost{
		ID:          77,
		JobTitle:    "Distributed Systems Go Engineer",
		Company:     "CloudCorp",
		Location:    "Berlin",
		SalaryRange: "€110k - €140k",
		Slug:        "distributed-systems-go-engineer-cloudcorp",
	}
	shareReq := formatLinkedInJobPost(cfg, sample)
	if err := client.shareJobPost(shareReq); err != nil {
		t.Fatalf("expected successful linkedin post, got error: %v", err)
	}
	if authHeader != "Bearer li-secret-token" {
		t.Fatalf("unexpected Authorization header: %s", authHeader)
	}
	if restliHeader != "2.0.0" {
		t.Fatalf("unexpected X-Restli-Protocol-Version header: %s", restliHeader)
	}
	if decoded["author"] != "urn:li:organization:123456" {
		t.Fatalf("unexpected author URN: %v", decoded["author"])
	}
	if !strings.Contains(shareReq.commentary, "https://golang.cafe/job/distributed-systems-go-engineer-cloudcorp") {
		t.Fatalf("unexpected commentary: %s", shareReq.commentary)
	}
}
