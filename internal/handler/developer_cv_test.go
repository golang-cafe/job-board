package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"

	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/golang-cafe/job-board/internal/user"
)

type mockDeveloperCVStore struct {
	devByID           map[string]developer.Developer
	devBySlug         map[string]developer.Developer
	trackedDownloads  int
	lastTrackedUserID string
}

func (m *mockDeveloperCVStore) DeveloperProfileByID(id string) (developer.Developer, error) {
	return m.devByID[id], nil
}

func (m *mockDeveloperCVStore) DeveloperProfileBySlug(slug string) (developer.Developer, error) {
	return m.devBySlug[slug], nil
}

func (m *mockDeveloperCVStore) SaveDeveloperCV(profileID string, cv []byte) error {
	dev := m.devByID[profileID]
	dev.CV = cv
	dev.HasCV = len(cv) > 0
	m.devByID[profileID] = dev
	m.devBySlug[dev.Slug] = dev
	return nil
}

func (m *mockDeveloperCVStore) TrackDeveloperCVDownload(id, profileID string, cv []byte, userID string) error {
	m.trackedDownloads++
	m.lastTrackedUserID = userID
	return nil
}

func createAuthenticatedRequest(t *testing.T, svr server.Server, req *http.Request, claims middleware.UserJWT) {
	t.Helper()
	rr := httptest.NewRecorder()
	sess, err := svr.SessionStore.Get(req, "____gc")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	tkn := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, signErr := tkn.SignedString(svr.GetJWTSigningKey())
	if signErr != nil {
		t.Fatalf("failed to sign jwt: %v", signErr)
	}
	sess.Values["jwt"] = signed
	if saveErr := sess.Save(req, rr); saveErr != nil {
		t.Fatalf("failed to save session: %v", saveErr)
	}
	for _, cookie := range rr.Result().Cookies() {
		req.AddCookie(cookie)
	}
}

func TestUploadAndDownloadDeveloperCV(t *testing.T) {
	jwtKey := []byte("0123456789abcdef0123456789abcdef")
	sessionStore := sessions.NewCookieStore([]byte("session-secret-key-0123456789abc"))
	cfg := config.Config{
		JwtSigningKey: jwtKey,
		SessionKey:    []byte("session-secret-key-0123456789abc"),
	}
	svr := server.NewServer(cfg, nil, mux.NewRouter(), nil, nil, sessionStore)

	dev := developer.Developer{
		ID:    "dev-1",
		Email: "dev@example.com",
		Slug:  "jane-doe-1",
	}
	store := &mockDeveloperCVStore{
		devByID:   map[string]developer.Developer{"dev-1": dev},
		devBySlug: map[string]developer.Developer{"jane-doe-1": dev},
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if fieldErr := mw.WriteField("profile_id", "dev-1"); fieldErr != nil {
		t.Fatalf("failed to write profile_id: %v", fieldErr)
	}
	fw, formErr := mw.CreateFormFile("cv", "resume.pdf")
	if formErr != nil {
		t.Fatalf("failed to create form file: %v", formErr)
	}
	pdfBytes := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF")
	if _, writeErr := fw.Write(pdfBytes); writeErr != nil {
		t.Fatalf("failed to write pdf bytes: %v", writeErr)
	}
	if closeErr := mw.Close(); closeErr != nil {
		t.Fatalf("failed to close multipart writer: %v", closeErr)
	}

	uploadReq := httptest.NewRequest(http.MethodPost, "/x/udcv", &body)
	uploadReq.Header.Set("Content-Type", mw.FormDataContentType())
	createAuthenticatedRequest(t, svr, uploadReq, middleware.UserJWT{
		UserID:      "user-dev-1",
		Email:       "dev@example.com",
		IsDeveloper: true,
		Type:        user.UserTypeDeveloper,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		},
	})

	uploadRR := httptest.NewRecorder()
	handleUploadDeveloperCV(svr, store, uploadRR, uploadReq)
	if uploadRR.Code != http.StatusOK {
		t.Fatalf("expected upload status 200, got %d", uploadRR.Code)
	}
	if !store.devByID["dev-1"].HasCV {
		t.Fatalf("expected developer CV to be stored")
	}

	downloadReq := httptest.NewRequest(http.MethodGet, "/developer/jane-doe-1/cv", nil)
	downloadReq = mux.SetURLVars(downloadReq, map[string]string{"slug": "jane-doe-1"})
	createAuthenticatedRequest(t, svr, downloadReq, middleware.UserJWT{
		UserID:      "user-rec-1",
		Email:       "recruiter@company.com",
		IsRecruiter: true,
		Type:        user.UserTypeRecruiter,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		},
	})

	downloadRR := httptest.NewRecorder()
	handleDownloadDeveloperCV(svr, store, downloadRR, downloadReq)
	if downloadRR.Code != http.StatusOK {
		t.Fatalf("expected download status 200, got %d", downloadRR.Code)
	}
	if got := downloadRR.Header().Get("Content-Type"); got != pdfContentTypeValue {
		t.Fatalf("expected Content-Type %s, got %s", pdfContentTypeValue, got)
	}
	if store.trackedDownloads != 1 || store.lastTrackedUserID != "user-rec-1" {
		t.Fatalf("expected CV download to be tracked for user-rec-1, got count=%d user=%s", store.trackedDownloads, store.lastTrackedUserID)
	}
}
