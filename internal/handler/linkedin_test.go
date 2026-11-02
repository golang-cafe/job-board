package handler

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-cafe/job-board/internal/job"
)

func TestResolveLinkedInAuthorURN(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"urn:li:person:abc123", "urn:li:person:abc123"},
		{"urn:li:organization:987654", "urn:li:organization:987654"},
		{"https://www.linkedin.com/company/golangcafe/", "urn:li:organization:golangcafe"},
		{"12345678", "urn:li:organization:12345678"},
	}
	for _, tc := range cases {
		got := resolveLinkedInAuthorURN(tc.in)
		if got != tc.want {
			t.Fatalf("resolveLinkedInAuthorURN(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPostJobToLinkedIn(t *testing.T) {
	var gotAuth, gotRestli, gotContentType string
	var gotPayload linkedInUGCPostRequest

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		gotRestli = r.Header.Get("X-Restli-Protocol-Version")
		gotContentType = r.Header.Get("Content-Type")
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed reading body: %v", err)
		}
		if err := json.Unmarshal(body, &gotPayload); err != nil {
			t.Fatalf("failed unmarshaling body: %v", err)
		}
		w.Header().Set("X-RestLi-Id", "urn:li:share:12345")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"urn:li:share:12345"}`))
	}))
	defer ts.Close()

	j := job.JobPost{
		ID:          42,
		JobTitle:    "Senior Go Engineer",
		Company:     "Acme Corp",
		Location:    "Remote",
		SalaryRange: "$140k - $180k",
		Slug:        "senior-go-engineer-acme-corp-42",
	}

	err := postJobToLinkedIn(ts.Client(), ts.URL, "test-token-123", "urn:li:organization:555", j, "golang", "https://", "golang.cafe")
	if err != nil {
		t.Fatalf("postJobToLinkedIn returned unexpected error: %v", err)
	}
	if gotAuth != "Bearer test-token-123" {
		t.Errorf("got Authorization %q, want %q", gotAuth, "Bearer test-token-123")
	}
	if gotRestli != "2.0.0" {
		t.Errorf("got X-Restli-Protocol-Version %q, want 2.0.0", gotRestli)
	}
	if gotContentType != "application/json" {
		t.Errorf("got Content-Type %q, want application/json", gotContentType)
	}
	if gotPayload.Author != "urn:li:organization:555" {
		t.Errorf("got Author %q, want urn:li:organization:555", gotPayload.Author)
	}
	if gotPayload.LifecycleState != "PUBLISHED" {
		t.Errorf("got LifecycleState %q, want PUBLISHED", gotPayload.LifecycleState)
	}
	if gotPayload.SpecificContent.ShareContent.ShareMediaCategory != "ARTICLE" {
		t.Errorf("got ShareMediaCategory %q, want ARTICLE", gotPayload.SpecificContent.ShareContent.ShareMediaCategory)
	}
	if len(gotPayload.SpecificContent.ShareContent.Media) != 1 {
		t.Fatalf("expected 1 media item, got %d", len(gotPayload.SpecificContent.ShareContent.Media))
	}
	if gotPayload.SpecificContent.ShareContent.Media[0].OriginalURL != "https://golang.cafe/job/senior-go-engineer-acme-corp-42" {
		t.Errorf("unexpected OriginalURL: %s", gotPayload.SpecificContent.ShareContent.Media[0].OriginalURL)
	}
	if !strings.Contains(gotPayload.SpecificContent.ShareContent.ShareCommentary.Text, "#golang #golangjobs") {
		t.Errorf("unexpected commentary text: %s", gotPayload.SpecificContent.ShareContent.ShareCommentary.Text)
	}
}
