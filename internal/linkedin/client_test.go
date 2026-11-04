package linkedin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func localClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	c, err := NewClient("synthetic-token", "urn:li:organization:123", "202610")
	if err != nil {
		t.Fatal(err)
	}
	base := server.Client().Transport
	c.http.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != endpoint {
			t.Fatalf("unexpected endpoint %s", r.URL)
		}
		clone := r.Clone(r.Context())
		u := *r.URL
		clone.URL = &u
		clone.URL.Scheme = "http"
		clone.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return base.RoundTrip(clone)
	})
	return c
}

func TestPostsRequestAndAcknowledgement(t *testing.T) {
	c := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/rest/posts" || r.Header.Get("Authorization") != "Bearer synthetic-token" || r.Header.Get("Linkedin-Version") != "202610" || r.Header.Get("X-Restli-Protocol-Version") != "2.0.0" {
			t.Error("incorrect request")
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["author"] != "urn:li:organization:123" || body["commentary"] != "A job\nhttps://example.invalid/job/a" || body["lifecycleState"] != "PUBLISHED" || body["visibility"] != "PUBLIC" {
			t.Error(body)
		}
		if body["distribution"].(map[string]interface{})["feedDistribution"] != "MAIN_FEED" {
			t.Error(body)
		}
		w.Header().Set("X-Restli-Id", "urn:li:share:1234")
		w.WriteHeader(201)
	})
	id, err := c.Post(context.Background(), "A job\nhttps://example.invalid/job/a")
	if err != nil || id != "urn:li:share:1234" {
		t.Fatal(id, err)
	}
}

func TestFailuresAreSanitizedAndNotRetried(t *testing.T) {
	for _, status := range []int{200, 201, 302, 401, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			c := localClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "https://example.invalid/leak")
				w.WriteHeader(status)
				fmt.Fprint(w, "synthetic-token private-content")
			})
			_, err := c.Post(context.Background(), "job")
			if err == nil || strings.Contains(err.Error(), "synthetic-token") || strings.Contains(err.Error(), "private-content") || calls != 1 {
				t.Fatal(err, calls)
			}
		})
	}
	c, _ := NewClient("synthetic-token", "urn:li:person:abc", "202610")
	c.http.Transport = transport(func(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("synthetic-token private-content") })
	_, err := c.Post(context.Background(), "job")
	if err == nil || strings.Contains(err.Error(), "synthetic-token") {
		t.Fatal(err)
	}
}

func TestConfigurationAndLength(t *testing.T) {
	for _, a := range [][3]string{{"", "urn:li:person:a", "202610"}, {"x", "https://linkedin.com/company/a", "202610"}, {"x", "urn:li:organization:1", "202613"}, {"x\r\ny", "urn:li:person:a", "202610"}} {
		if _, err := NewClient(a[0], a[1], a[2]); err == nil {
			t.Fatal(a)
		}
	}
	c, _ := NewClient("synthetic", "urn:li:person:a", "202610")
	for _, text := range []string{"", strings.Repeat("a", 3001)} {
		if _, err := c.Post(context.Background(), text); err == nil {
			t.Fatal("length accepted")
		}
	}
}
