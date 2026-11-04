package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/email"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

func TestTwitterSchedulerAuthAndConfiguration(t *testing.T) {
	svr := server.NewServer(config.Config{MachineToken: "fixture-machine"}, nil, mux.NewRouter(), nil, email.Client{}, sessions.NewCookieStore([]byte("synthetic-twitter-fixture-only!!")))
	for _, route := range []string{"legacy", "jobs"} {
		h := TriggerTwitterJobsScheduler(svr)
		if route == "legacy" {
			h = TriggerTwitterScheduler(svr, nil)
		}
		for _, token := range []string{"", "incorrect", "fixture-machine"} {
			t.Run(route+"-"+token, func(t *testing.T) {
				r := httptest.NewRequest(http.MethodPost, "/x/task/twitter-jobs-scheduler", nil)
				r.Header.Set("x-machine-token", token)
				w := httptest.NewRecorder()
				h(w, r)
				want := 401
				if token == "fixture-machine" {
					want = 503
				}
				if w.Code != want {
					t.Fatalf("status%d want%d", w.Code, want)
				}
			})
		}
	}
	empty := server.NewServer(config.Config{}, nil, mux.NewRouter(), nil, email.Client{}, sessions.NewCookieStore([]byte("synthetic-twitter-fixture-only!!")))
	w := httptest.NewRecorder()
	TriggerTwitterJobsScheduler(empty)(w, httptest.NewRequest("POST", "/x/task/twitter-jobs-scheduler", nil))
	if w.Code != 503 {
		t.Fatal("empty machine token enabled scheduler", w.Code)
	}
}
