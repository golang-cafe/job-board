package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/database"
	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/email"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	_ "github.com/lib/pq"
)

func photoCookie(t *testing.T, svr server.Server, r *http.Request, expired bool) {
	t.Helper()
	expires := time.Now().Add(time.Hour).Unix()
	if expired {
		expires = time.Now().Add(-time.Hour).Unix()
	}
	claims := middleware.UserJWT{Email: "fixture@example.invalid", IsDeveloper: true, StandardClaims: jwt.StandardClaims{ExpiresAt: expires}}
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

type photoDevFixture struct{ saves int }

func (p *photoDevFixture) DeveloperProfileByEmail(string) (developer.Developer, error) {
	return developer.Developer{}, nil
}
func (p *photoDevFixture) SaveDeveloperProfile(developer.Developer) error { p.saves++; return nil }
func (p *photoDevFixture) SaveTokenSignOn(string, string, string) error   { p.saves++; return nil }

func TestPhotoHTTPAndSQL(t *testing.T) {
	url := os.Getenv("PHOTO_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set PHOTO_TEST_DATABASE_URL to an isolated PostgreSQL database")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	// Temporary tables shadow real tables, even when reusing an existing database.
	_, err = db.Exec(`CREATE TEMP TABLE image(id text PRIMARY KEY, bytes bytea, media_type text);
	CREATE TEMP TABLE developer_profile(image_id text, id text, name text, location text, linkedin_url text, hourly_rate bigint, bio text, available boolean, updated_at timestamp, skills text, search_status text, role_level text);
	CREATE TEMP TABLE email_subscribers(email text, token text, created_at timestamp);`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/108_developer_photo.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	localEmail, err := email.NewClient("", "", "", "fixture@example.invalid", "fixture@example.invalid", "Fixture", true)
	if err != nil {
		t.Fatal(err)
	}
	svr := server.NewServer(config.Config{JwtSigningKey: []byte("synthetic-photo-test-signing-only")}, db, mux.NewRouter(), nil, localEmail, sessions.NewCookieStore([]byte("synthetic-photo-fixture-session!!")))
	im := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			if x < 64 {
				im.Set(x, y, color.White)
			} else {
				im.Set(x, y, color.Black)
			}
		}
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, im); err != nil {
		t.Fatal(err)
	}
	original := encoded.Bytes()
	id, err := database.SaveMedia(db, database.Media{Bytes: original, MediaType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO developer_profile(image_id) VALUES($1)`, id); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, query, auth string
		status            int
	}{
		{"anonymous", "", "", 200}, {"malformed-size", "?w=bad&h=bad", "", 200},
		{"resized", "?w=64&h=64", "", 200}, {"negative-size", "?w=-1&h=1", "", 400},
		{"huge-size", "?w=999999&h=999999", "", 400}, {"logged-in", "", "valid", 200},
		{"expired", "", "expired", 200}, {"forged-cookie", "", "forged", 200},
		{"malformed-jwt", "", "malformed", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/media/"+id+tc.query, nil)
			r = mux.SetURLVars(r, map[string]string{"id": id})
			if tc.auth == "valid" || tc.auth == "expired" {
				photoCookie(t, svr, r, tc.auth == "expired")
			}
			if tc.auth == "forged" {
				r.AddCookie(&http.Cookie{Name: "____gc", Value: "invalid"})
			}
			if tc.auth == "malformed" {
				sess, _ := svr.SessionStore.Get(r, "____gc")
				sess.Values["jwt"] = "not-a-jwt"
				cookieResponse := httptest.NewRecorder()
				if err := sess.Save(r, cookieResponse); err != nil {
					t.Fatal(err)
				}
				for _, c := range cookieResponse.Result().Cookies() {
					r.AddCookie(c)
				}
			}
			w := httptest.NewRecorder()
			RetrieveMediaPageHandler(svr)(w, r)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if w.Header().Get("Cache-Control") != "private, no-store" || !strings.Contains(w.Header().Get("Vary"), "Cookie") {
				t.Fatal("unsafe cache headers", w.Header())
			}
			if tc.status == 200 {
				if tc.auth == "valid" {
					if !bytes.Equal(w.Body.Bytes(), original) {
						t.Fatal("logged-in image changed")
					}
				} else {
					if bytes.Equal(w.Body.Bytes(), original) {
						t.Fatal("original exposed")
					}
					blurred, _, err := image.Decode(bytes.NewReader(w.Body.Bytes()))
					if err != nil {
						t.Fatal(err)
					}
					if tc.query == "" {
						r, _, _, _ := blurred.At(63, 64).RGBA()
						if r == 65535 {
							t.Fatal("boundary was not blurred")
						}
					}
				}
			}
		})
	}
	t.Run("company-unchanged", func(t *testing.T) {
		company, err := database.SaveMedia(db, database.Media{Bytes: original, MediaType: "image/png"})
		if err != nil {
			t.Fatal(err)
		}
		r := mux.SetURLVars(httptest.NewRequest("GET", "/media/"+company, nil), map[string]string{"id": company})
		w := httptest.NewRecorder()
		RetrieveMediaPageHandler(svr)(w, r)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), original) {
			t.Fatal("company image changed")
		}
		if w.Header().Get("Cache-Control") != "max-age=31536000" {
			t.Fatal("company cache behavior changed")
		}
	})
	t.Run("signup-direct-image-id-rejected", func(t *testing.T) {
		fixture := &photoDevFixture{}
		payload := map[string]interface{}{"email": "fixture@example.invalid", "linkedin_url": "https://linkedin.com/in/fixture", "search_status": "casually-looking", "role_level": "senior", "detected_location_id": "fixture", "hourly_rate": "50", "profile_image_id": id}
		b, _ := json.Marshal(payload)
		w := httptest.NewRecorder()
		SaveDeveloperProfileHandler(svr, fixture, fixture)(w, httptest.NewRequest("POST", "/signup", bytes.NewReader(b)))
		if w.Code != 400 || fixture.saves != 0 || !strings.Contains(w.Body.String(), "visible face") {
			t.Fatalf("bypass: %d %s saves%d", w.Code, w.Body, fixture.saves)
		}
	})
	facePath := os.Getenv("PHOTO_TEST_FACE")
	for _, tc := range []struct {
		name, query string
		face        bool
		status      int
	}{
		{"developer-no-face", "?purpose=developer", false, 400}, {"company-upload", "", false, 200},
		{"invalid-crop", "?purpose=developer&x=99999&w=20&h=20", false, 400},
		{"negative-crop", "?purpose=developer&x=-1&w=20&h=20", false, 400},
		{"developer-face", "?purpose=developer", true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := original
			if tc.face {
				if facePath == "" {
					t.Skip("PHOTO_TEST_FACE not set")
				}
				data, err = os.ReadFile(facePath)
				if err != nil {
					t.Fatal(err)
				}
			}
			var b bytes.Buffer
			writer := multipart.NewWriter(&b)
			part, _ := writer.CreateFormFile("image", "fixture.png")
			part.Write(data)
			writer.Close()
			r := httptest.NewRequest("POST", "/x/s/m"+tc.query, &b)
			r.Header.Set("Content-Type", writer.FormDataContentType())
			w := httptest.NewRecorder()
			SaveMediaPageHandler(svr)(w, r)
			if w.Code != tc.status {
				t.Fatalf("%d: %s", w.Code, w.Body)
			}
			if tc.face && w.Code == 200 {
				var res struct {
					ID string `json:"id"`
				}
				json.Unmarshal(w.Body.Bytes(), &res)
				protected, err := database.IsDeveloperMedia(db, res.ID)
				if err != nil || !protected {
					t.Fatal("new developer upload is public", err)
				}
				if err = validateDeveloperMedia(svr, res.ID); err != nil {
					t.Fatal(err)
				}
				t.Run("signup-photo-accepted-local-email", func(t *testing.T) {
					fixture := &photoDevFixture{}
					payload := map[string]interface{}{"email": "fixture@example.invalid", "linkedin_url": "https://linkedin.com/in/fixture", "search_status": "casually-looking", "role_level": "senior", "detected_location_id": "fixture", "hourly_rate": "50", "profile_image_id": res.ID}
					b, _ := json.Marshal(payload)
					w := httptest.NewRecorder()
					SaveDeveloperProfileHandler(svr, fixture, fixture)(w, httptest.NewRequest("POST", "/signup", bytes.NewReader(b)))
					if w.Code != 200 || fixture.saves != 2 {
						t.Fatalf("signup %d saves%d: %s", w.Code, fixture.saves, w.Body)
					}
				})
				t.Run("profile-update-revalidates-image", func(t *testing.T) {
					if _, err := db.Exec(`INSERT INTO developer_profile(id,image_id) VALUES('photo-profile',$1)`, id); err != nil {
						t.Fatal(err)
					}
					for _, imageID := range []string{res.ID, id} {
						payload := map[string]interface{}{"id": "photo-profile", "email": "fixture@example.invalid", "linkedin_url": "https://linkedin.com/in/fixture", "search_status": "casually-looking", "role_level": "senior", "hourly_rate": "50", "profile_image_id": imageID}
						b, _ := json.Marshal(payload)
						r := httptest.NewRequest("PUT", "/profile", bytes.NewReader(b))
						photoCookie(t, svr, r, false)
						w := httptest.NewRecorder()
						UpdateDeveloperProfileHandler(svr, developer.NewRepository(db))(w, r)
						want := 200
						if imageID == id {
							want = 400
						}
						if w.Code != want {
							t.Fatalf("update %d want%d: %s", w.Code, want, w.Body)
						}
					}
					var stored string
					if err := db.QueryRow(`SELECT image_id FROM developer_profile WHERE id='photo-profile'`).Scan(&stored); err != nil || stored != res.ID {
						t.Fatal("invalid photo overwrote profile", stored, err)
					}
				})
			}
		})
	}
	t.Run("missing-image-fails-closed", func(t *testing.T) {
		if err := validateDeveloperMedia(svr, "missing"); err == nil {
			t.Fatal("missing image accepted")
		}
		if err := validateDeveloperMedia(svr, ""); err == nil {
			t.Fatal("empty image accepted")
		}
	})
}
