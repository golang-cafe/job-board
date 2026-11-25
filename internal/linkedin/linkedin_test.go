package linkedin

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorURNFromPageURL(t *testing.T) {
	testCases := []struct {
		name    string
		pageURL string
		want    string
		wantErr bool
	}{
		{name: "company page url", pageURL: "https://www.linkedin.com/company/12345678", want: "urn:li:organization:12345678"},
		{name: "company page url trailing slash", pageURL: "https://www.linkedin.com/company/12345678/", want: "urn:li:organization:12345678"},
		{name: "full urn passthrough", pageURL: "urn:li:organization:12345678", want: "urn:li:organization:12345678"},
		{name: "company vanity url has no numeric id", pageURL: "https://www.linkedin.com/company/golang-cafe", wantErr: true},
		{name: "personal profile url unsupported", pageURL: "https://www.linkedin.com/in/johndoe", wantErr: true},
		{name: "empty url", pageURL: "", wantErr: true},
		{name: "not a url", pageURL: "linkedin", wantErr: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AuthorURNFromPageURL(tc.pageURL)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for page url %q, got author %q", tc.pageURL, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for page url %q: %v", tc.pageURL, err)
			}
			if got != tc.want {
				t.Fatalf("expected author %q for page url %q, got %q", tc.want, tc.pageURL, got)
			}
		})
	}
}

func TestBuildSharePayload(t *testing.T) {
	payload := BuildSharePayload("urn:li:organization:12345678", "Senior Go Developer with Acme - Remote | $100k\n\nhttps://golang.cafe/job/senior-go-developer", "https://golang.cafe/job/senior-go-developer")
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("unable to marshal payload: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unable to unmarshal payload: %v", err)
	}
	if decoded["author"] != "urn:li:organization:12345678" {
		t.Errorf("expected author urn, got %v", decoded["author"])
	}
	if decoded["lifecycleState"] != "PUBLISHED" {
		t.Errorf("expected lifecycleState PUBLISHED, got %v", decoded["lifecycleState"])
	}
	specificContent := decoded["specificContent"].(map[string]interface{})
	shareContent := specificContent["com.linkedin.ugc.ShareContent"].(map[string]interface{})
	commentary := shareContent["shareCommentary"].(map[string]interface{})
	if !strings.Contains(commentary["text"].(string), "Senior Go Developer with Acme") {
		t.Errorf("expected commentary to contain the job text, got %v", commentary["text"])
	}
	if shareContent["shareMediaCategory"] != "ARTICLE" {
		t.Errorf("expected shareMediaCategory ARTICLE, got %v", shareContent["shareMediaCategory"])
	}
	media := shareContent["media"].([]interface{})
	if len(media) != 1 {
		t.Fatalf("expected exactly one media item, got %d", len(media))
	}
	mediaItem := media[0].(map[string]interface{})
	if mediaItem["status"] != "READY" {
		t.Errorf("expected media status READY, got %v", mediaItem["status"])
	}
	if mediaItem["originalUrl"] != "https://golang.cafe/job/senior-go-developer" {
		t.Errorf("expected media originalUrl to be the job url, got %v", mediaItem["originalUrl"])
	}
	visibility := decoded["visibility"].(map[string]interface{})
	if visibility["com.linkedin.ugc.MemberNetworkVisibility"] != "PUBLIC" {
		t.Errorf("expected public visibility, got %v", visibility)
	}
}

func TestSharePost(t *testing.T) {
	var gotAuth, gotRestliVersion, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/ugcPosts" {
			t.Errorf("expected path /v2/ugcPosts, got %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		gotRestliVersion = r.Header.Get("X-Restli-Protocol-Version")
		body, _ := ioutil.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	client, err := NewClient("test-token", "https://www.linkedin.com/company/12345678")
	if err != nil {
		t.Fatalf("unable to create client: %v", err)
	}
	client.BaseURL = srv.URL
	err = client.SharePost(context.Background(), "job text", "https://golang.cafe/job/senior-go-developer")
	if err != nil {
		t.Fatalf("unexpected share error: %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("expected Authorization header Bearer test-token, got %q", gotAuth)
	}
	if gotRestliVersion != "2.0.0" {
		t.Errorf("expected X-Restli-Protocol-Version 2.0.0, got %q", gotRestliVersion)
	}
	if !strings.Contains(gotBody, `"author":"urn:li:organization:12345678"`) {
		t.Errorf("expected request body to carry the author urn, got %s", gotBody)
	}
}

func TestSharePostError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Invalid access token"}`))
	}))
	defer srv.Close()

	client, err := NewClient("bad-token", "https://www.linkedin.com/company/12345678")
	if err != nil {
		t.Fatalf("unable to create client: %v", err)
	}
	client.BaseURL = srv.URL
	err = client.SharePost(context.Background(), "job text", "https://golang.cafe/job/senior-go-developer")
	if err == nil {
		t.Fatal("expected an error for a 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention the status, got %v", err)
	}
}
