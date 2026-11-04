package handler

import (
	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/email"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"net/http/httptest"
	"testing"
)

func TestLinkedinTaskAuthenticationAndMissingConfiguration(t *testing.T) {
	for _, key := range []string{"fixture-machine", ""} {
		svr := server.NewServer(config.Config{MachineToken: key}, nil, mux.NewRouter(), nil, email.Client{}, sessions.NewCookieStore([]byte("synthetic-linkedin-fixture-only")))
		for _, token := range []string{"", "wrong", "fixture-machine"} {
			r := httptest.NewRequest("POST", "/x/task/linkedin-share-jobs", nil)
			r.Header.Set("x-machine-token", token)
			w := httptest.NewRecorder()
			TriggerLinkedinShareJobs(svr)(w, r)
			want := 401
			if token == key {
				want = 503
			}
			if w.Code != want {
				t.Fatal(key, token, w.Code, want)
			}
		}
	}
}
