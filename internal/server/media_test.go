package server

import (
	"net/http/httptest"
	"testing"
)

func TestMediaCachePolicy(t *testing.T) {
	for _, policy := range []string{"", "private, no-store"} {
		w := httptest.NewRecorder()
		if policy != "" {
			w.Header().Set("Cache-Control", policy)
		}
		Server{}.MEDIA(w, 200, []byte("fixture"), "image/png")
		want := policy
		if want == "" {
			want = "max-age=31536000"
		}
		if w.Header().Get("Cache-Control") != want {
			t.Fatal(w.Header())
		}
	}
}
