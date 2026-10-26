package twitterapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dghubble/oauth1"
)

func TestClientPostTweetSendsV2PayloadAndOAuthHeader(t *testing.T) {
	var gotBody string
	var gotAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"data":{"id":"1","text":"hello"}}`)
	}))
	defer server.Close()

	config := oauth1.NewConfig("consumer-key", "consumer-secret")
	token := oauth1.NewToken("access-token", "access-token-secret")
	client := &Client{
		httpClient: config.Client(oauth1.NoContext, token),
		endpoint:   server.URL,
	}

	if err := client.PostTweet(context.Background(), "hello"); err != nil {
		t.Fatalf("PostTweet returned error: %v", err)
	}
	if gotBody != `{"text":"hello"}` {
		t.Fatalf("unexpected request body: %s", gotBody)
	}
	if !strings.HasPrefix(gotAuth, "OAuth ") {
		t.Fatalf("missing OAuth authorization header: %q", gotAuth)
	}
}

func TestClientPostTweetRejectsEmptyText(t *testing.T) {
	client := &Client{}
	if err := client.PostTweet(context.Background(), "   "); err == nil {
		t.Fatal("expected error for empty text")
	}
}
