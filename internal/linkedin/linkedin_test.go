package linkedin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShareJobPostSendsRestliHeadersAndUGCPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Restli-Protocol-Version") != "2.0.0" {
			t.Fatalf("missing X-Restli-Protocol-Version header")
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("invalid Authorization header")
		}
		w.Header().Set("X-RestLi-Id", "urn:li:share:999")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := NewClient("test-token", "urn:li:organization:12345", "https://linkedin.com/company/golang-cafe", "golang.cafe")
	c.Endpoint = srv.URL

	urn, err := c.ShareJobPost(JobPostItem{
		ID:          42,
		JobTitle:    "Senior Go Engineer",
		Company:     "Acme Corp",
		Location:    "Remote",
		SalaryRange: "$160k - $190k",
		Slug:        "senior-go-engineer-acme-42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urn != "urn:li:share:999" {
		t.Fatalf("expected urn:li:share:999, got %q", urn)
	}
}
