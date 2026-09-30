package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveLinkedInAuthorURN(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "   ", want: ""},
		{name: "existing organization URN", input: "urn:li:organization:12345", want: "urn:li:organization:12345"},
		{name: "existing person URN", input: "urn:li:person:abcde", want: "urn:li:person:abcde"},
		{name: "company page URL", input: "https://www.linkedin.com/company/golang-cafe/", want: "urn:li:organization:golang-cafe"},
		{name: "person profile URL", input: "https://www.linkedin.com/in/gopher-dev/", want: "urn:li:person:gopher-dev"},
		{name: "raw organization id", input: "987654", want: "urn:li:organization:987654"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveLinkedInAuthorURN(tc.input); got != tc.want {
				t.Fatalf("resolveLinkedInAuthorURN(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestPostLinkedInShareArticle(t *testing.T) {
	var gotAuth, gotProtocol, gotContentType string
	var gotPayload linkedInUGCPostRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		gotProtocol = r.Header.Get("X-Restli-Protocol-Version")
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"urn:li:share:123"}`))
	}))
	defer srv.Close()

	err := postLinkedInShare(
		context.Background(),
		srv.Client(),
		srv.URL,
		"secret-token",
		"https://www.linkedin.com/company/golang-cafe",
		"Senior Go Engineer with Acme - Remote | $150k\n\n#golang #golangjobs\n\nhttps://golang.cafe/job/senior-go-engineer",
		"https://golang.cafe/job/senior-go-engineer",
		"Senior Go Engineer with Acme",
	)
	if err != nil {
		t.Fatalf("postLinkedInShare returned unexpected error: %v", err)
	}

	if gotAuth != "Bearer secret-token" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer secret-token")
	}
	if gotProtocol != "2.0.0" {
		t.Fatalf("X-Restli-Protocol-Version = %q, want 2.0.0", gotProtocol)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotPayload.Author != "urn:li:organization:golang-cafe" {
		t.Fatalf("Author = %q, want urn:li:organization:golang-cafe", gotPayload.Author)
	}
	if gotPayload.LifecycleState != "PUBLISHED" {
		t.Fatalf("LifecycleState = %q, want PUBLISHED", gotPayload.LifecycleState)
	}
	if gotPayload.Visibility.MemberNetworkVisibility != "PUBLIC" {
		t.Fatalf("Visibility = %q, want PUBLIC", gotPayload.Visibility.MemberNetworkVisibility)
	}
	share := gotPayload.SpecificContent.ShareContent
	if share.ShareMediaCategory != "ARTICLE" {
		t.Fatalf("ShareMediaCategory = %q, want ARTICLE", share.ShareMediaCategory)
	}
	if len(share.Media) != 1 || share.Media[0].OriginalURL != "https://golang.cafe/job/senior-go-engineer" {
		t.Fatalf("unexpected media payload: %+v", share.Media)
	}
}

func TestPostLinkedInShareErrorHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"access denied"}`))
	}))
	defer srv.Close()

	err := postLinkedInShare(context.Background(), srv.Client(), srv.URL, "token", "urn:li:organization:1", "text", "", "")
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}
