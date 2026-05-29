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

func TestFormatSocialJobPost(t *testing.T) {
	post := &job.JobPost{
		JobTitle:    "Senior Go Engineer",
		Company:     "Acme",
		Location:    "Remote",
		SalaryRange: "$100k - $150k",
		Slug:        "senior-go-engineer",
	}
	got := formatSocialJobPost(post, "Go", "https://", "jobs.example.com")
	want := "Senior Go Engineer with Acme - Remote | $100k - $150k\n\n#Go #Gojobs\n\nhttps://jobs.example.com/job/senior-go-engineer"
	if got != want {
		t.Fatalf("unexpected post text:\nwant %q\ngot  %q", want, got)
	}
}

func TestTwitterClientPostTweetBuildsSignedV2Request(t *testing.T) {
	var requestBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/2/tweets" {
			t.Fatalf("path = %s, want /2/tweets", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		auth := r.Header.Get("Authorization")
		for _, part := range []string{
			`oauth_consumer_key="client-key"`,
			`oauth_nonce="fixed-nonce"`,
			`oauth_signature_method="HMAC-SHA1"`,
			`oauth_timestamp="1700000000"`,
			`oauth_token="access-token"`,
			`oauth_version="1.0"`,
			`oauth_signature=`,
		} {
			if !strings.Contains(auth, part) {
				t.Fatalf("Authorization header missing %s: %s", part, auth)
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	client := twitterClient{
		accessToken:       "access-token",
		accessTokenSecret: "access-secret",
		clientKey:         "client-key",
		clientSecret:      "client-secret",
		endpoint:          srv.URL + "/2/tweets",
		httpClient:        srv.Client(),
		nonce:             func() string { return "fixed-nonce" },
		now:               func() time.Time { return time.Unix(1700000000, 0) },
	}
	if err := client.postTweet(context.Background(), "hello world"); err != nil {
		t.Fatal(err)
	}
	if requestBody["text"] != "hello world" {
		t.Fatalf("tweet text = %q", requestBody["text"])
	}
}

func TestLinkedinClientShareArticleBuildsUGCPost(t *testing.T) {
	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("X-Restli-Protocol-Version"); got != "2.0.0" {
			t.Fatalf("X-Restli-Protocol-Version = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	client := linkedinClient{
		accessToken: "token",
		authorURN:   "urn:li:organization:123",
		endpoint:    srv.URL,
		httpClient:  srv.Client(),
	}
	if err := client.shareArticle(context.Background(), "post text", "Job title", "https://jobs.example.com/job/job-title"); err != nil {
		t.Fatal(err)
	}
	if body["author"] != "urn:li:organization:123" {
		t.Fatalf("author = %v", body["author"])
	}
	content := body["specificContent"].(map[string]interface{})["com.linkedin.ugc.ShareContent"].(map[string]interface{})
	if content["shareMediaCategory"] != "ARTICLE" {
		t.Fatalf("shareMediaCategory = %v", content["shareMediaCategory"])
	}
	commentary := content["shareCommentary"].(map[string]interface{})
	if commentary["text"] != "post text" {
		t.Fatalf("commentary text = %v", commentary["text"])
	}
}
