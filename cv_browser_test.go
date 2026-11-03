package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/email"
	"github.com/golang-cafe/job-board/internal/handler"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	view "github.com/golang-cafe/job-board/internal/template"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	_ "github.com/lib/pq"
)

func TestCVBrowserIntegration(t *testing.T) {
	script := os.Getenv("CV_BROWSER_SCRIPT")
	dsn := os.Getenv("CV_TEST_DATABASE_URL")
	if script == "" || dsn == "" {
		t.Skip("set CV_BROWSER_SCRIPT, CV_NODE and CV_TEST_DATABASE_URL for browser integration")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`INSERT INTO developer_profile(id,email) VALUES ('browser-profile','browser@example.invalid')`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`DELETE FROM developer_profile WHERE id='browser-profile'`)
	router := mux.NewRouter()
	templates := view.NewTemplate(staticFS)
	svr := server.NewServer(config.Config{JwtSigningKey: []byte("synthetic-browser-fixture-only")}, db, router, templates, email.Client{}, sessions.NewCookieStore([]byte("synthetic-session-fixture-only!!!")))
	repo := developer.NewRepository(db)
	router.HandleFunc("/x/developer-cv", handler.UploadDeveloperCV(svr, repo)).Methods("POST")
	router.HandleFunc("/x/developer-cv/{id}", handler.DownloadDeveloperCV(svr, repo)).Methods("GET")
	// This fixture-only login does not exist in production routes.
	router.HandleFunc("/fixture/{role}", func(w http.ResponseWriter, r *http.Request) {
		role := mux.Vars(r)["role"]
		claims := middleware.UserJWT{Email: "browser@example.invalid", UserID: "browser-user", IsDeveloper: role == "developer", IsRecruiter: role == "recruiter", IsAdmin: role == "admin", StandardClaims: jwt.StandardClaims{ExpiresAt: time.Now().Add(time.Hour).Unix()}}
		token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(svr.GetJWTSigningKey())
		sess, _ := svr.SessionStore.Get(r, "____gc")
		sess.Values["jwt"] = token
		sess.Save(r, w)
		w.WriteHeader(204)
	})
	for _, page := range []struct{ path, name string }{{"/edit", "edit-developer-profile.html"}, {"/profile", "view-developer-profile.html"}} {
		page := page
		router.HandleFunc(page.path, func(w http.ResponseWriter, r *http.Request) {
			var has bool
			if err := db.QueryRow(`SELECT cv IS NOT NULL FROM developer_profile WHERE id='browser-profile'`).Scan(&has); err != nil {
				t.Error(err)
				return
			}
			claims, _ := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			// Match production's empty metadata slices, rather than absent map keys.
			data := map[string]interface{}{"DeveloperProfile": developer.Developer{ID: "browser-profile", Name: "Synthetic Developer", HasCV: has}, "LoggedUser": claims, "IsUserRecruiter": claims != nil && claims.IsRecruiter, "IsUserAdmin": claims != nil && claims.IsAdmin, "PrimaryColor": "#167542", "SecondaryColor": "#125c35"}
			for _, key := range []string{"DeveloperExperiences", "DeveloperEducation", "DeveloperGithubProjects"} {
				data[key] = []developer.DeveloperMetadata{}
			}
			if err := templates.Render(w, 200, page.name, data); err != nil {
				t.Error(err)
			}
		})
	}
	httpServer := httptest.NewServer(router)
	defer httpServer.Close()
	cmd := exec.Command(os.Getenv("CV_NODE"), script, httpServer.URL)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("browser: %v\n%s", err, out)
	} else {
		t.Log(string(out))
	}
}
