package handler

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

type cvFixtureStore struct {
	email, profile, user string
	data                 []byte
	err                  error
	calls                int
}

func (s *cvFixtureStore) SaveCV(_ context.Context, email string, data []byte) error {
	s.calls++
	s.email = email
	s.data = data
	return s.err
}
func (s *cvFixtureStore) DownloadCV(_ context.Context, profile, user string) ([]byte, error) {
	s.calls++
	s.profile = profile
	s.user = user
	return s.data, s.err
}

func cvSession(t *testing.T, svr server.Server, r *http.Request, role string) {
	t.Helper()
	if role == "anonymous" {
		return
	}
	claims := middleware.UserJWT{Email: "fixture@example.invalid", UserID: "fixture-user", IsDeveloper: role == "developer", IsRecruiter: role == "recruiter", IsAdmin: role == "admin", StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(time.Hour).Unix()}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(svr.GetJWTSigningKey())
	if err != nil {
		t.Fatal(err)
	}
	sess, _ := svr.SessionStore.Get(r, "____gc")
	sess.Values["jwt"] = token
	w := httptest.NewRecorder()
	if err = sess.Save(r, w); err != nil {
		t.Fatal(err)
	}
	for _, c := range w.Result().Cookies() {
		r.AddCookie(c)
	}
}

func TestDeveloperCVDownloads(t *testing.T) {
	for _, role := range []string{"anonymous", "developer", "recruiter", "admin"} {
		t.Run(role, func(t *testing.T) {
			svr := server.Server{SessionStore: sessions.NewCookieStore([]byte("synthetic-test-session-key-only!!"))}
			store := &cvFixtureStore{data: []byte("%PDF-1.4\nsynthetic")}
			r := httptest.NewRequest("GET", "/x/developer-cv/fixture-profile", nil)
			r = mux.SetURLVars(r, map[string]string{"id": "fixture-profile"})
			cvSession(t, svr, r, role)
			w := httptest.NewRecorder()
			DownloadDeveloperCV(svr, store)(w, r)
			allowed := role == "recruiter" || role == "admin"
			if allowed {
				if w.Code != 200 || store.calls != 1 || store.profile != "fixture-profile" || store.user != "fixture-user" {
					t.Fatalf("download %d %+v", w.Code, store)
				}
				if w.Header().Get("Cache-Control") != "private, no-store" {
					t.Fatal("CV can be cached")
				}
			} else if store.calls != 0 || w.Code == 200 {
				t.Fatalf("unauthorized download %d", w.Code)
			}
		})
	}
	for _, err := range []error{sql.ErrNoRows, errors.New("audit insert failed")} {
		svr := server.Server{SessionStore: sessions.NewCookieStore([]byte("synthetic-test-session-key-only!!"))}
		store := &cvFixtureStore{data: []byte("SECRET FIXTURE"), err: err}
		r := httptest.NewRequest("GET", "/x/developer-cv/id", nil)
		cvSession(t, svr, r, "recruiter")
		w := httptest.NewRecorder()
		DownloadDeveloperCV(svr, store)(w, r)
		if w.Code == 200 || bytes.Contains(w.Body.Bytes(), store.data) {
			t.Fatal("failed audit released CV")
		}
	}
}

func TestDeveloperCVUploads(t *testing.T) {
	for _, tc := range []struct {
		name, role, header string
		data               []byte
		ok                 bool
	}{
		{"pdf", "developer", "1", []byte("%PDF-1.4\nfixture"), true},
		{"replacement", "developer", "1", []byte("%PDF-1.7\nreplacement"), true},
		{"not-pdf", "developer", "1", []byte("<script>fixture</script>"), false},
		{"empty", "developer", "1", nil, false},
		{"oversize", "developer", "1", append([]byte("%PDF-"), make([]byte, maxCVBytes)...), false},
		{"csrf", "developer", "", []byte("%PDF-1.4"), false},
		{"anonymous", "anonymous", "1", []byte("%PDF-1.4"), false},
		{"recruiter", "recruiter", "1", []byte("%PDF-1.4"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			writer := multipart.NewWriter(&b)
			part, _ := writer.CreateFormFile("cv", "fixture.pdf")
			part.Write(tc.data)
			writer.WriteField("email", "victim@example.invalid")
			writer.Close()
			r := httptest.NewRequest("POST", "/x/developer-cv", &b)
			r.Header.Set("Content-Type", writer.FormDataContentType())
			r.Header.Set("X-CV-Upload", tc.header)
			svr := server.Server{SessionStore: sessions.NewCookieStore([]byte("synthetic-test-session-key-only!!"))}
			cvSession(t, svr, r, tc.role)
			store := &cvFixtureStore{}
			w := httptest.NewRecorder()
			UploadDeveloperCV(svr, store)(w, r)
			if tc.ok {
				if w.Code != 204 || store.email != "fixture@example.invalid" || !bytes.Equal(store.data, tc.data) {
					t.Fatalf("upload %d %+v", w.Code, store)
				}
			} else if w.Code == 204 || store.calls != 0 {
				t.Fatalf("invalid upload accepted: %d", w.Code)
			}
		})
	}
}
