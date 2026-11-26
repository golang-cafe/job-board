package linkedin

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildArticleSharePayload(t *testing.T) {
	post := BuildArticleSharePayload("urn:li:organization:1234567", "Go Developer with Acme - Remote | $100k\n\n#golang #golangjobs\n\nhttps://golang.cafe/job/go-developer", "https://golang.cafe/job/go-developer", "Go Developer", "Acme - Remote | $100k")
	body, err := json.Marshal(post)
	if err != nil {
		t.Fatalf("unable to marshal payload: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unable to unmarshal payload: %v", err)
	}
	if decoded["author"] != "urn:li:organization:1234567" {
		t.Errorf("author = %v, want urn:li:organization:1234567", decoded["author"])
	}
	if decoded["lifecycleState"] != "PUBLISHED" {
		t.Errorf("lifecycleState = %v, want PUBLISHED", decoded["lifecycleState"])
	}
	content, ok := decoded["specificContent"].(map[string]interface{})["com.linkedin.ugc.ShareContent"].(map[string]interface{})
	if !ok {
		t.Fatalf("specificContent missing com.linkedin.ugc.ShareContent: %s", body)
	}
	if content["shareMediaCategory"] != "ARTICLE" {
		t.Errorf("shareMediaCategory = %v, want ARTICLE", content["shareMediaCategory"])
	}
	media, ok := content["media"].([]interface{})
	if !ok || len(media) != 1 {
		t.Fatalf("media = %v, want one article", content["media"])
	}
	article, ok := media[0].(map[string]interface{})
	if !ok {
		t.Fatalf("media[0] is not an object: %v", media[0])
	}
	if article["status"] != "READY" {
		t.Errorf("media status = %v, want READY", article["status"])
	}
	if article["originalUrl"] != "https://golang.cafe/job/go-developer" {
		t.Errorf("originalUrl = %v, want the job URL", article["originalUrl"])
	}
	if article["title"].(map[string]interface{})["text"] != "Go Developer" {
		t.Errorf("title = %v, want Go Developer", article["title"])
	}
	visibility, ok := decoded["visibility"].(map[string]interface{})["com.linkedin.ugc.MemberNetworkVisibility"]
	if !ok || visibility != "PUBLIC" {
		t.Errorf("visibility = %v, want PUBLIC", decoded["visibility"])
	}
}

func TestResolveAuthorURN(t *testing.T) {
	tests := []struct {
		name      string
		pageURL   string
		authorURN string
		want      string
		wantErr   bool
	}{
		{name: "explicit author URN wins", pageURL: "https://www.linkedin.com/company/123/", authorURN: "urn:li:person:8675309", want: "urn:li:person:8675309"},
		{name: "raw URN as page URL", pageURL: "urn:li:organization:1234567", want: "urn:li:organization:1234567"},
		{name: "company URL with numeric id", pageURL: "https://www.linkedin.com/company/1234567/", want: "urn:li:organization:1234567"},
		{name: "company URL without trailing slash", pageURL: "https://www.linkedin.com/company/1234567", want: "urn:li:organization:1234567"},
		{name: "personal profile URL", pageURL: "https://www.linkedin.com/in/johndoe/", want: "urn:li:person:johndoe"},
		{name: "nothing configured", wantErr: true},
		{name: "bad author URN", authorURN: "not-a-urn", wantErr: true},
		{name: "company vanity name", pageURL: "https://www.linkedin.com/company/golang-cafe/", wantErr: true},
		{name: "non-linkedin URL", pageURL: "https://example.com/company/123/", wantErr: true},
		{name: "unknown path", pageURL: "https://www.linkedin.com/groups/123/", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveAuthorURN(tc.pageURL, tc.authorURN)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ResolveAuthorURN(%q, %q) = %q, want error", tc.pageURL, tc.authorURN, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveAuthorURN(%q, %q) returned error: %v", tc.pageURL, tc.authorURN, err)
			}
			if got != tc.want {
				t.Errorf("ResolveAuthorURN(%q, %q) = %q, want %q", tc.pageURL, tc.authorURN, got, tc.want)
			}
		})
	}
}

func TestShareArticle(t *testing.T) {
	var gotAuth, gotProtocol, gotContentType string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotProtocol = r.Header.Get("X-Restli-Protocol-Version")
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = ioutil.ReadAll(r.Body)
		w.Header().Set("X-RestLi-Id", "urn:li:share:123456789")
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	// Point the client at the test server.
	origURL := ugcPostsURL
	ugcPostsURL = server.URL
	defer func() { ugcPostsURL = origURL }()

	client := NewClient("test-token", "urn:li:organization:1234567")
	id, err := client.ShareArticle(context.Background(), "Go Developer with Acme", "https://golang.cafe/job/go-developer", "Go Developer", "Acme - Remote")
	if err != nil {
		t.Fatalf("ShareArticle returned error: %v", err)
	}
	if id != "urn:li:share:123456789" {
		t.Errorf("ShareArticle id = %q, want urn:li:share:123456789", id)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want Bearer test-token", gotAuth)
	}
	if gotProtocol != "2.0.0" {
		t.Errorf("X-Restli-Protocol-Version = %q, want 2.0.0", gotProtocol)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if !strings.Contains(string(gotBody), "Go Developer") {
		t.Errorf("request body does not contain the job title: %s", gotBody)
	}
}

func TestShareArticleNon201(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"Invalid access token"}`)
	}))
	defer server.Close()

	origURL := ugcPostsURL
	ugcPostsURL = server.URL
	defer func() { ugcPostsURL = origURL }()

	client := NewClient("bad-token", "urn:li:organization:1234567")
	_, err := client.ShareArticle(context.Background(), "text", "https://golang.cafe/job/x", "Title", "Desc")
	if err == nil {
		t.Fatal("ShareArticle with a 401 response should return an error")
	}
	if !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "Invalid access token") {
		t.Errorf("error = %q, want it to carry the status and LinkedIn message", err.Error())
	}
}
