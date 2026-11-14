package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/job"
)

func TestResolveLinkedinAuthorURN(t *testing.T) {
	if got := resolveLinkedinAuthorURN("urn:li:person:abc123", ""); got != "urn:li:person:abc123" {
		t.Fatalf("expected urn:li:person:abc123, got %s", got)
	}
	if got := resolveLinkedinAuthorURN("", "https://www.linkedin.com/company/golang-cafe/"); got != "urn:li:organization:golang-cafe" {
		t.Fatalf("expected urn:li:organization:golang-cafe, got %s", got)
	}
	if got := resolveLinkedinAuthorURN("987654", ""); got != "urn:li:organization:987654" {
		t.Fatalf("expected urn:li:organization:987654, got %s", got)
	}
}

func TestPostJobToLinkedin(t *testing.T) {
	var receivedHeader string
	var receivedProtocol string
	var receivedBody linkedinUGCPostRequest

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("Authorization")
		receivedProtocol = r.Header.Get("X-Restli-Protocol-Version")
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"urn:li:share:12345"}`))
	}))
	defer ts.Close()

	origEndpoint := linkedinUGCPostsEndpoint
	linkedinUGCPostsEndpoint = ts.URL
	defer func() { linkedinUGCPostsEndpoint = origEndpoint }()

	cfg := config.Config{
		LinkedinAccessToken: "test-linkedin-token",
		LinkedinPageURL:     "https://www.linkedin.com/company/golang-cafe",
		SiteJobCategory:     "golang",
		URLProtocol:         "https://",
		SiteHost:            "golang.cafe",
	}
	sampleJob := &job.JobPost{
		ID:          42,
		JobTitle:    "Senior Go Engineer",
		Company:     "Acme Corp",
		Location:    "Remote",
		SalaryRange: "$150k - $180k",
		Slug:        "senior-go-engineer-acme-corp",
	}

	if err := PostJobToLinkedin(ts.Client(), cfg, sampleJob); err != nil {
		t.Fatalf("PostJobToLinkedin failed: %v", err)
	}

	if receivedHeader != "Bearer test-linkedin-token" {
		t.Errorf("expected Bearer test-linkedin-token, got %s", receivedHeader)
	}
	if receivedProtocol != "2.0.0" {
		t.Errorf("expected X-Restli-Protocol-Version 2.0.0, got %s", receivedProtocol)
	}
	if receivedBody.Author != "urn:li:organization:golang-cafe" {
		t.Errorf("expected author urn:li:organization:golang-cafe, got %s", receivedBody.Author)
	}
	if receivedBody.Visibility.MemberNetworkVisibility != "PUBLIC" {
		t.Errorf("expected PUBLIC visibility, got %s", receivedBody.Visibility.MemberNetworkVisibility)
	}
}
