package handler

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/email"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

type mockDeveloperCVRepo struct {
	profiles  map[string]developer.Developer
	bySlug    map[string]developer.Developer
	downloads []developer.DeveloperCVDownload
}

func newMockDeveloperCVRepo() *mockDeveloperCVRepo {
	return &mockDeveloperCVRepo{
		profiles:  make(map[string]developer.Developer),
		bySlug:    make(map[string]developer.Developer),
		downloads: make([]developer.DeveloperCVDownload, 0),
	}
}

func (m *mockDeveloperCVRepo) DeveloperProfileByID(id string) (developer.Developer, error) {
	dev, ok := m.profiles[id]
	if !ok {
		return developer.Developer{}, sql.ErrNoRows
	}
	return dev, nil
}

func (m *mockDeveloperCVRepo) SaveDeveloperCV(profileID string, cv []byte) error {
	dev, ok := m.profiles[profileID]
	if !ok {
		return sql.ErrNoRows
	}
	dev.CV = cv
	dev.HasCV = len(cv) > 0
	m.profiles[profileID] = dev
	if dev.Slug != "" {
		m.bySlug[dev.Slug] = dev
	}
	return nil
}

func (m *mockDeveloperCVRepo) GetDeveloperCVBySlug(slugStr string) (developer.Developer, error) {
	dev, ok := m.bySlug[slugStr]
	if !ok {
		return developer.Developer{}, sql.ErrNoRows
	}
	return dev, nil
}

func (m *mockDeveloperCVRepo) TrackDeveloperCVDownload(download developer.DeveloperCVDownload) error {
	m.downloads = append(m.downloads, download)
	return nil
}

func newTestServerForCV() (server.Server, []byte) {
	jwtKey := []byte("test-secret-signing-key-32-bytes!")
	sessionKey := []byte("test-session-secret-key-32-bytes")
	cfg := config.Config{
		JwtSigningKey: jwtKey,
		SessionKey:    sessionKey,
		Env:           "test",
	}
	store := sessions.NewCookieStore(sessionKey)
	svr := server.NewServer(cfg, nil, mux.NewRouter(), nil, email.Client{}, store)
	return svr, jwtKey
}

func attachUserSession(t *testing.T, svr server.Server, jwtKey []byte, claims middleware.UserJWT) []*http.Cookie {
	t.Helper()
	if claims.StandardClaims.ExpiresAt == 0 {
		claims.StandardClaims = jwt.StandardClaims{
			ExpiresAt: time.Now().UTC().Add(1 * time.Hour).Unix(),
		}
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtKey)
	if err != nil {
		t.Fatalf("unable to sign test jwt: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	sess, err := svr.SessionStore.Get(req, "____gc")
	if err != nil {
		t.Fatalf("unable to create session: %v", err)
	}
	sess.Values["jwt"] = signed
	if err := sess.Save(req, rec); err != nil {
		t.Fatalf("unable to save session: %v", err)
	}
	return rec.Result().Cookies()
}

func TestValidateDeveloperCVBytes(t *testing.T) {
	validPDF := []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n")
	if err := validateDeveloperCVBytes(validPDF); err != nil {
		t.Fatalf("expected valid PDF to pass validation, got: %v", err)
	}

	if err := validateDeveloperCVBytes(nil); err == nil {
		t.Fatal("expected empty CV to fail validation")
	}

	if err := validateDeveloperCVBytes([]byte("not a pdf file")); err == nil {
		t.Fatal("expected non-PDF bytes to fail validation")
	}

	polyglotPDF := []byte("%PDF-1.4\n<html><script>alert(1)</script></html>\n%%EOF")
	if err := validateDeveloperCVBytes(polyglotPDF); err == nil {
		t.Fatal("expected polyglot HTML-in-PDF payload to fail validation")
	}

	oversizedPDF := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("A"), MaxDeveloperCVSize)...)
	if err := validateDeveloperCVBytes(oversizedPDF); err == nil {
		t.Fatal("expected >5MB PDF to fail validation")
	}
}

func TestDecodeAndValidateBase64CV(t *testing.T) {
	validPDF := []byte("%PDF-1.4\n%test pdf content\n%%EOF")
	encoded := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(validPDF)
	decoded, err := decodeAndValidateBase64CV(encoded)
	if err != nil {
		t.Fatalf("unexpected error decoding data URL PDF: %v", err)
	}
	if !bytes.Equal(decoded, validPDF) {
		t.Fatalf("decoded PDF mismatch")
	}

	badDataURI := "data:text/html;base64," + base64.StdEncoding.EncodeToString(validPDF)
	if _, err := decodeAndValidateBase64CV(badDataURI); err == nil {
		t.Fatal("expected non-PDF MIME data URI to fail validation")
	}

	emptyDecoded, err := decodeAndValidateBase64CV("   ")
	if err != nil || emptyDecoded != nil {
		t.Fatalf("expected empty string to return nil, nil; got %v, %v", emptyDecoded, err)
	}
}

func TestUploadDeveloperCVHandler(t *testing.T) {
	svr, jwtKey := newTestServerForCV()
	repo := newMockDeveloperCVRepo()
	dev := developer.Developer{
		ID:    "27charProfileID000000000001",
		Email: "dev@example.com",
		Slug:  "jane-doe-123",
		Name:  "Jane Doe",
	}
	repo.profiles[dev.ID] = dev
	repo.bySlug[dev.Slug] = dev

	handlerFn := UploadDeveloperCVHandler(svr, repo)
	validPDF := []byte("%PDF-1.4\n%uploaded developer cv\n%%EOF")

	// 1. Multipart upload by profile owner
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("id", dev.ID)
	part, err := writer.CreateFormFile("cv", "resume.pdf")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	_, _ = part.Write(validPDF)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/x/udp/cv", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for _, c := range attachUserSession(t, svr, jwtKey, middleware.UserJWT{
		UserID:      "27charUserID000000000000001",
		Email:       "dev@example.com",
		IsDeveloper: true,
		Type:        "developer",
	}) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	handlerFn.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200 for owner multipart CV upload, got %d: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Equal(repo.profiles[dev.ID].CV, validPDF) {
		t.Fatal("expected CV bytes to be stored in repository")
	}

	// 2. JSON base64 upload by another user (forbidden)
	payload, _ := json.Marshal(map[string]string{
		"id": dev.ID,
		"cv": "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(validPDF),
	})
	reqForbidden := httptest.NewRequest(http.MethodPost, "/x/udp/cv", bytes.NewReader(payload))
	reqForbidden.Header.Set("Content-Type", "application/json")
	for _, c := range attachUserSession(t, svr, jwtKey, middleware.UserJWT{
		UserID:      "27charUserID000000000000002",
		Email:       "other@example.com",
		IsDeveloper: true,
		Type:        "developer",
	}) {
		reqForbidden.AddCookie(c)
	}
	recForbidden := httptest.NewRecorder()
	handlerFn.ServeHTTP(recForbidden, reqForbidden)
	if recForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected status 403 for non-owner CV upload, got %d", recForbidden.Code)
	}
}

func TestDownloadDeveloperCVHandlerAndTracking(t *testing.T) {
	svr, jwtKey := newTestServerForCV()
	repo := newMockDeveloperCVRepo()
	validPDF := []byte("%PDF-1.4\n%recruiter downloadable cv\n%%EOF")
	dev := developer.Developer{
		ID:    "27charProfileID000000000001",
		Email: "dev@example.com",
		Slug:  "jane-doe-123",
		Name:  "Jane Doe",
		CV:    validPDF,
		HasCV: true,
	}
	repo.profiles[dev.ID] = dev
	repo.bySlug[dev.Slug] = dev

	router := mux.NewRouter()
	router.HandleFunc("/developer/{slug}/cv", DownloadDeveloperCVHandler(svr, repo)).Methods(http.MethodGet)

	// 1. Recruiter downloads CV -> 200 OK + tracked in developer_cv_download + security headers
	recruiterUserID := "27charRecruiterID0000000001"
	req := httptest.NewRequest(http.MethodGet, "/developer/jane-doe-123/cv", nil)
	for _, c := range attachUserSession(t, svr, jwtKey, middleware.UserJWT{
		UserID:      recruiterUserID,
		Email:       "recruiter@company.com",
		IsRecruiter: true,
		Type:        "recruiter",
	}) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for recruiter CV download, got %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), validPDF) {
		t.Fatal("downloaded CV bytes do not match stored CV")
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options=nosniff, got %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'none'" {
		t.Fatalf("expected Content-Security-Policy=\"default-src 'none'\", got %q", got)
	}
	if len(repo.downloads) != 1 {
		t.Fatalf("expected 1 tracked CV download record, got %d", len(repo.downloads))
	}
	if repo.downloads[0].DeveloperProfileID != dev.ID || repo.downloads[0].UserID != recruiterUserID {
		t.Fatalf("unexpected download tracking metadata: %+v", repo.downloads[0])
	}
	if !bytes.Equal(repo.downloads[0].CV, validPDF) {
		t.Fatal("tracked download CV bytes do not match")
	}

	// 2. Another developer (non-owner, non-recruiter, non-admin) cannot download CV -> 403 Forbidden
	reqOtherDev := httptest.NewRequest(http.MethodGet, "/developer/jane-doe-123/cv", nil)
	for _, c := range attachUserSession(t, svr, jwtKey, middleware.UserJWT{
		UserID:      "27charOtherDevID00000000001",
		Email:       "otherdev@example.com",
		IsDeveloper: true,
		Type:        "developer",
	}) {
		reqOtherDev.AddCookie(c)
	}
	recOtherDev := httptest.NewRecorder()
	router.ServeHTTP(recOtherDev, reqOtherDev)
	if recOtherDev.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-owner developer, got %d", recOtherDev.Code)
	}

	// 3. Developer without CV -> 404 Not Found with "CV not uploaded"
	noCVDev := developer.Developer{
		ID:    "27charProfileID000000000002",
		Email: "nocv@example.com",
		Slug:  "no-cv-dev",
		Name:  "No CV",
		HasCV: false,
	}
	repo.profiles[noCVDev.ID] = noCVDev
	repo.bySlug[noCVDev.Slug] = noCVDev

	reqNoCV := httptest.NewRequest(http.MethodGet, "/developer/no-cv-dev/cv", nil)
	for _, c := range attachUserSession(t, svr, jwtKey, middleware.UserJWT{
		UserID:      recruiterUserID,
		Email:       "recruiter@company.com",
		IsRecruiter: true,
		Type:        "recruiter",
	}) {
		reqNoCV.AddCookie(c)
	}
	recNoCV := httptest.NewRecorder()
	router.ServeHTTP(recNoCV, reqNoCV)
	if recNoCV.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when CV is not uploaded, got %d", recNoCV.Code)
	}
	if !strings.Contains(recNoCV.Body.String(), "CV not uploaded") {
		t.Fatalf("expected 'CV not uploaded' message, got %q", recNoCV.Body.String())
	}
}
