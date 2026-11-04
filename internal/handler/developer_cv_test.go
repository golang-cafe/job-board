package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/email"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/golang-cafe/job-board/internal/user"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

type mockDevCVRepo struct {
	devByID           developer.Developer
	devBySlug         developer.Developer
	savedID           string
	savedCV           []byte
	trackedDownloadID string
	trackedProfileID  string
	trackedCV         []byte
	trackedUserID     string
}

func (m *mockDevCVRepo) DeveloperProfileByID(id string) (developer.Developer, error) {
	return m.devByID, nil
}

func (m *mockDevCVRepo) SaveDeveloperCV(id string, cv []byte) error {
	m.savedID = id
	m.savedCV = cv
	return nil
}

func (m *mockDevCVRepo) GetDeveloperCVBySlug(slug string) (developer.Developer, error) {
	return m.devBySlug, nil
}

func (m *mockDevCVRepo) TrackDeveloperCVDownload(id, developerProfileID string, cv []byte, userID string) error {
	m.trackedDownloadID = id
	m.trackedProfileID = developerProfileID
	m.trackedCV = cv
	m.trackedUserID = userID
	return nil
}

func newTestServerWithAuth() (server.Server, *sessions.CookieStore, []byte) {
	jwtKey := []byte("0123456789abcdef0123456789abcdef")
	store := sessions.NewCookieStore([]byte("session-secret-key-12345678901234"))
	svr := server.NewServer(
		config.Config{
			JwtSigningKey: jwtKey,
		},
		nil,
		mux.NewRouter(),
		nil,
		email.Client{},
		store,
	)
	return svr, store, jwtKey
}

func attachSessionJWT(t *testing.T, r *http.Request, store *sessions.CookieStore, jwtKey []byte, claims middleware.UserJWT) {
	t.Helper()
	rec := httptest.NewRecorder()
	sess, err := store.Get(r, "____gc")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	claims.StandardClaims = jwt.StandardClaims{
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtKey)
	if err != nil {
		t.Fatalf("failed to sign jwt: %v", err)
	}
	sess.Values["jwt"] = signed
	if err := sess.Save(r, rec); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}
	for _, c := range rec.Result().Cookies() {
		r.AddCookie(c)
	}
}

func samplePDFBytes() []byte {
	return []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n")
}

func TestValidateAndDecodeDeveloperCV(t *testing.T) {
	validPDF := samplePDFBytes()
	if err := validateDeveloperCV(validPDF); err != nil {
		t.Fatalf("expected valid PDF to pass validation, got: %v", err)
	}

	if err := validateDeveloperCV([]byte("not a pdf")); err == nil {
		t.Fatal("expected non-PDF bytes to fail validation")
	}

	oversized := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("A"), maxDeveloperCVSize)...)
	if err := validateDeveloperCV(oversized); err == nil {
		t.Fatal("expected >5MB PDF to fail validation")
	}

	dataURI := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(validPDF)
	decoded, err := decodeAndValidateBase64CV(dataURI)
	if err != nil {
		t.Fatalf("expected data-URI base64 PDF to decode and validate: %v", err)
	}
	if !bytes.Equal(decoded, validPDF) {
		t.Fatal("decoded PDF bytes did not match original")
	}
}

func TestUploadDeveloperCVHandler(t *testing.T) {
	svr, store, jwtKey := newTestServerWithAuth()
	repo := &mockDevCVRepo{
		devByID: developer.Developer{
			ID:    "dev-1",
			Email: "dev@example.com",
		},
	}

	pdfBytes := samplePDFBytes()
	payload, _ := json.Marshal(map[string]string{
		"id": "dev-1",
		"cv": base64.StdEncoding.EncodeToString(pdfBytes),
	})

	req := httptest.NewRequest(http.MethodPost, "/x/udp/cv", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	attachSessionJWT(t, req, store, jwtKey, middleware.UserJWT{
		UserID:      "user-dev-1",
		Email:       "dev@example.com",
		IsDeveloper: true,
		Type:        user.UserTypeDeveloper,
	})

	rec := httptest.NewRecorder()
	UploadDeveloperCVHandler(svr, repo).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d (%s)", rec.Code, rec.Body.String())
	}
	if repo.savedID != "dev-1" || !bytes.Equal(repo.savedCV, pdfBytes) {
		t.Fatalf("expected CV to be saved for dev-1")
	}
}

func TestDownloadDeveloperCVHandler_PermissionsAndTracking(t *testing.T) {
	svr, store, jwtKey := newTestServerWithAuth()
	pdfBytes := samplePDFBytes()
	repo := &mockDevCVRepo{
		devBySlug: developer.Developer{
			ID:    "dev-1",
			Email: "dev@example.com",
			Slug:  "jane-doe-123",
			CV:    pdfBytes,
			HasCV: true,
		},
	}

	router := mux.NewRouter()
	router.HandleFunc("/developer/{slug}/cv", DownloadDeveloperCVHandler(svr, repo)).Methods(http.MethodGet)

	// 1. Another developer cannot download CV -> 403 Forbidden
	otherDevReq := httptest.NewRequest(http.MethodGet, "/developer/jane-doe-123/cv", nil)
	attachSessionJWT(t, otherDevReq, store, jwtKey, middleware.UserJWT{
		UserID:      "user-dev-2",
		Email:       "other@example.com",
		IsDeveloper: true,
		Type:        user.UserTypeDeveloper,
	})
	otherDevRec := httptest.NewRecorder()
	router.ServeHTTP(otherDevRec, otherDevReq)
	if otherDevRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-owner developer, got %d", otherDevRec.Code)
	}

	// 2. Recruiter can download CV and download is tracked in developer_cv_download -> 200 OK
	recReq := httptest.NewRequest(http.MethodGet, "/developer/jane-doe-123/cv", nil)
	attachSessionJWT(t, recReq, store, jwtKey, middleware.UserJWT{
		UserID:      "recruiter-99",
		Email:       "hr@company.com",
		IsRecruiter: true,
		Type:        user.UserTypeRecruiter,
	})
	recRec := httptest.NewRecorder()
	router.ServeHTTP(recRec, recReq)
	if recRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for recruiter, got %d", recRec.Code)
	}
	if gotCT := recRec.Header().Get("Content-Type"); gotCT != "application/pdf" {
		t.Fatalf("expected Content-Type application/pdf, got %q", gotCT)
	}
	if !strings.Contains(recRec.Header().Get("Content-Disposition"), "jane-doe-123-cv.pdf") {
		t.Fatalf("unexpected Content-Disposition: %q", recRec.Header().Get("Content-Disposition"))
	}
	if !bytes.Equal(recRec.Body.Bytes(), pdfBytes) {
		t.Fatal("downloaded PDF body did not match stored CV")
	}
	if repo.trackedProfileID != "dev-1" || repo.trackedUserID != "recruiter-99" || !bytes.Equal(repo.trackedCV, pdfBytes) || repo.trackedDownloadID == "" {
		t.Fatalf("expected download tracking to record profile=dev-1 and user=recruiter-99, got profile=%q user=%q id=%q",
			repo.trackedProfileID, repo.trackedUserID, repo.trackedDownloadID)
	}
}
