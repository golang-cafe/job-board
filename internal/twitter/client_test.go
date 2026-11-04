package twitter

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func fixtureClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient("fixture-consumer", "fixture-consumer-secret", "fixture-access", "fixture-access-secret")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCreatePostSignedJSON(t *testing.T) {
	var endpoint string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/2/tweets" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("wrong v2 request", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text != "fixture job\nhttps://example.invalid/job/one" {
			t.Error("bad JSON payload", err)
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "OAuth ") {
			t.Error("not user-context OAuth")
		}
		params := url.Values{}
		for _, field := range strings.Split(strings.TrimPrefix(header, "OAuth "), ",") {
			kv := strings.SplitN(strings.TrimSpace(field), "=", 2)
			if len(kv) != 2 {
				continue
			}
			v, err := url.QueryUnescape(strings.Trim(kv[1], "\""))
			if err != nil {
				t.Error(err)
			}
			params.Set(kv[0], v)
		}
		signature := params.Get("oauth_signature")
		params.Del("oauth_signature")
		if params.Get("oauth_token") != "fixture-access" || params.Get("oauth_consumer_key") != "fixture-consumer" || params.Get("oauth_signature_method") != "HMAC-SHA1" {
			t.Error("wrong OAuth credentials/method")
		}
		base := "POST&" + url.QueryEscape(endpoint) + "&" + url.QueryEscape(params.Encode())
		mac := hmac.New(sha1.New, []byte("fixture-consumer-secret&fixture-access-secret"))
		mac.Write([]byte(base))
		if signature != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
			t.Error("invalid OAuth signature")
		}
		w.WriteHeader(201)
		w.Write([]byte(`{"data":{"id":"fixture-post"}}`))
	}))
	defer ts.Close()
	endpoint = ts.URL + "/2/tweets"
	c := fixtureClient(t)
	c.endpoint = endpoint
	id, err := c.Post(context.Background(), "fixture job\nhttps://example.invalid/job/one")
	if err != nil || id != "fixture-post" {
		t.Fatal(id, err)
	}
}

func TestCreatePostFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"forbidden", 403, "fixture-secret"}, {"rate-limit", 429, "fixture-secret"}, {"server-error", 500, "fixture-secret"},
		{"wrong-status", 200, `{"data":{"id":"one"}}`}, {"missing-id", 201, `{"data":{}}`}, {"bad-json", 201, "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int32
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer ts.Close()
			c := fixtureClient(t)
			c.endpoint = ts.URL
			id, err := c.Post(context.Background(), "fixture")
			if err == nil || id != "" || strings.Contains(err.Error(), "fixture-secret") || atomic.LoadInt32(&calls) != 1 {
				t.Fatal("unsafe error/retry", id, err, calls)
			}
		})
	}
}

func TestRedirectDoesNotForwardCredentials(t *testing.T) {
	var forwarded int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&forwarded, 1) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer origin.Close()
	c := fixtureClient(t)
	c.endpoint = origin.URL
	if _, err := c.Post(context.Background(), "fixture"); err == nil || atomic.LoadInt32(&forwarded) != 0 {
		t.Fatal("redirect was followed", err)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("fixture-transport-secret")
}

func TestTransportErrorRedaction(t *testing.T) {
	c := fixtureClient(t)
	c.http.Transport = failingTransport{}
	_, err := c.Post(context.Background(), "fixture")
	if err == nil || strings.Contains(err.Error(), "fixture-transport-secret") {
		t.Fatal(err)
	}
	if _, err := NewClient("", "", "", ""); err == nil {
		t.Fatal("missing configuration accepted")
	}
}
