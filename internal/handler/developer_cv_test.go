package handler

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

// fakeDeveloperCVRepo is an in-memory stand-in for developer.Repository.
// It records downloads so tests can assert the audit behaviour, and it can
// be told to fail audit writes to prove downloads fail closed.
type fakeDeveloperCVRepo struct {
	profiles  map[string]developer.Developer
	bySlug    map[string]developer.Developer
	downloads []developer.DeveloperCVDownload
	failAudit bool
}

func newFakeDeveloperCVRepo() *fakeDeveloperCVRepo {
	return &fakeDeveloperCVRepo{
		profiles:  make(map[string]developer.Developer),
		bySlug:    make(map[string]developer.Developer),
		downloads: make([]developer.DeveloperCVDownload, 0),
	}
}

func (m *fakeDeveloperCVRepo) addProfile(dev developer.Developer) {
	if len(dev.CV) > 0 {
		dev.HasCV = true
	}
	m.profiles[dev.ID] = dev
	if dev.Slug != "" {
		m.bySlug[dev.Slug] = dev
	}
}

func (m *fakeDeveloperCVRepo) DeveloperProfileByID(id string) (developer.Developer, error) {
	dev, ok := m.profiles[id]
	if !ok {
		return developer.Developer{}, sql.ErrNoRows
	}
	return dev, nil
}

func (m *fakeDeveloperCVRepo) SaveDeveloperCV(profileID string, cv []byte) error {
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

func (m *fakeDeveloperCVRepo) DeleteDeveloperCV(profileID string) error {
	dev, ok := m.profiles[profileID]
	if !ok {
		return sql.ErrNoRows
	}
	dev.CV = nil
	dev.HasCV = false
	m.profiles[profileID] = dev
	if dev.Slug != "" {
		m.bySlug[dev.Slug] = dev
	}
	return nil
}

func (m *fakeDeveloperCVRepo) GetDeveloperCVBySlug(slugStr string) (developer.Developer, error) {
	dev, ok := m.bySlug[slugStr]
	if !ok {
		return developer.Developer{}, sql.ErrNoRows
	}
	return dev, nil
}

func (m *fakeDeveloperCVRepo) TrackDeveloperCVDownload(download developer.DeveloperCVDownload) error {
	if m.failAudit {
		return errors.New("audit write failed")
	}
	m.downloads = append(m.downloads, download)
	return nil
}

func (m *fakeDeveloperCVRepo) GetDeveloperCVDownloads(profileID string, limit int) ([]developer.DeveloperCVDownloadRecord, error) {
	records := []developer.DeveloperCVDownloadRecord{}
	for i := len(m.downloads) - 1; i >= 0 && len(records) < limit; i-- {
		d := m.downloads[i]
		if d.DeveloperProfileID == profileID {
			records = append(records, developer.DeveloperCVDownloadRecord{
				ID:                 d.ID,
				DeveloperProfileID: d.DeveloperProfileID,
				UserID:             d.UserID,
				DownloadedAt:       d.DownloadedAt,
			})
		}
	}
	return records, nil
}

func (m *fakeDeveloperCVRepo) GetDeveloperCVDownloadCount(profileID string) (int, error) {
	count := 0
	for _, d := range m.downloads {
		if d.DeveloperProfileID == profileID {
			count++
		}
	}
	return count, nil
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

func addCookies(req *http.Request, cookies []*http.Cookie) {
	for _, c := range cookies {
		req.AddCookie(c)
	}
}

func testPDF() []byte {
	return []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n")
}

func TestValidateDeveloperCVBytes(t *testing.T) {
	if err := validateDeveloperCVBytes(testPDF()); err != nil {
		t.Fatalf("expected valid PDF to pass validation, got: %v", err)
	}
	if err := validateDeveloperCVBytes(nil); err == nil {
		t.Fatal("expected empty CV to fail validation")
	}
	if err := validateDeveloperCVBytes([]byte("not a pdf file")); err == nil {
		t.Fatal("expected non-PDF bytes to fail validation")
	}
	// Truncated PDF: valid header but no EOF trailer.
	if err := validateDeveloperCVBytes([]byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n")); err == nil {
		t.Fatal("expected PDF without EOF trailer to fail validation")
	}
	// Header without a version digit.
	if err := validateDeveloperCVBytes([]byte("%PDF-\n%%EOF")); err == nil {
		t.Fatal("expected PDF header without version to fail validation")
	}
	polyglotPDF := []byte("%PDF-1.4\n<html><script>alert(1)</script></html>\n%%EOF")
	if err := validateDeveloperCVBytes(polyglotPDF); err == nil {
		t.Fatal("expected polyglot HTML-in-PDF payload to fail validation")
	}
	embeddedJSPDF := []byte("%PDF-1.4\n1 0 obj\n<< /Type /Action /S /JavaScript /JS (app.alert(1)) >>\nendobj\n%%EOF")
	if err := validateDeveloperCVBytes(embeddedJSPDF); err == nil {
		t.Fatal("expected PDF with embedded /JavaScript action to fail validation")
	}
	launchPDF := []byte("%PDF-1.4\n1 0 obj\n<< /Type /Action /S /Launch >>\nendobj\n%%EOF")
	if err := validateDeveloperCVBytes(launchPDF); err == nil {
		t.Fatal("expected PDF with /Launch action to fail validation")
	}
	oversizedPDF := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("A"), MaxDeveloperCVSize)...)
	if err := validateDeveloperCVBytes(oversizedPDF); err == nil {
		t.Fatal("expected >5MB PDF to fail validation")
	}
	// Exactly 5MB is allowed.
	atLimit := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("A"), MaxDeveloperCVSize-len("%PDF-1.4\n")-len("%%EOF\n"))...)
	atLimit = append(atLimit, []byte("%%EOF\n")...)
	if len(atLimit) != MaxDeveloperCVSize {
		t.Fatalf("test fixture is %d bytes, expected exactly MaxDeveloperCVSize", len(atLimit))
	}
	if err := validateDeveloperCVBytes(atLimit); err != nil {
		t.Fatalf("expected exactly-5MB PDF to pass validation, got: %v", err)
	}
}

func TestDecodeAndValidateBase64CV(t *testing.T) {
	encoded := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(testPDF())
	decoded, err := decodeAndValidateBase64CV(encoded)
	if err != nil {
		t.Fatalf("unexpected error decoding data URL PDF: %v", err)
	}
	if !bytes.Equal(decoded, testPDF()) {
		t.Fatal("decoded PDF mismatch")
	}
	plain := base64.StdEncoding.EncodeToString(testPDF())
	decoded, err = decodeAndValidateBase64CV(plain)
	if err != nil {
		t.Fatalf("unexpected error decoding plain base64 PDF: %v", err)
	}
	if !bytes.Equal(decoded, testPDF()) {
		t.Fatal("decoded PDF mismatch")
	}
	if decoded, err := decodeAndValidateBase64CV(""); err != nil || decoded != nil {
		t.Fatalf("expected empty input to decode to nil, nil, got %v, %v", decoded, err)
	}
	wrongMime := "data:image/png;base64," + base64.StdEncoding.EncodeToString(testPDF())
	if _, err := decodeAndValidateBase64CV(wrongMime); err == nil {
		t.Fatal("expected non-PDF data URI to fail validation")
	}
	if _, err := decodeAndValidateBase64CV("!!!not-base64!!!"); err == nil {
		t.Fatal("expected invalid base64 to fail validation")
	}
	oversized := base64.StdEncoding.EncodeToString(append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("A"), MaxDeveloperCVSize)...))
	if _, err := decodeAndValidateBase64CV(oversized); err == nil {
		t.Fatal("expected oversized base64 CV to fail validation")
	}
}

func TestIsValidOptionalLinkedInURL(t *testing.T) {
	if !isValidOptionalLinkedInURL("") {
		t.Fatal("expected empty LinkedIn URL to be valid (optional)")
	}
	if !isValidOptionalLinkedInURL("https://www.linkedin.com/in/someone") {
		t.Fatal("expected valid LinkedIn URL to be valid")
	}
	if !isValidOptionalLinkedInURL("https://linkedin.com/in/someone") {
		t.Fatal("expected bare linkedin.com URL to be valid")
	}
	for _, bad := range []string{"not a url", "https://example.com/in/someone", "http://www.linkedin.com/in/someone", "https://www.linkedin.com.evil.com/in/x"} {
		if isValidOptionalLinkedInURL(bad) {
			t.Fatalf("expected %q to be an invalid LinkedIn URL", bad)
		}
	}
}

func TestSanitizePDFFileName(t *testing.T) {
	cases := map[string]string{
		"john-doe-123":   "john-doe-123-cv.pdf",
		"John Doe/Smith": "John-Doe-Smith-cv.pdf",
		`a"b\c`:          "a-b-c-cv.pdf",
		"":               "developer-cv.pdf",
		"---":            "developer-cv.pdf",
		"../etc/passwd":  "etc-passwd-cv.pdf",
	}
	for in, want := range cases {
		if got := sanitizePDFFileName(in); got != want {
			t.Fatalf("sanitizePDFFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUploadDeveloperCVHandler(t *testing.T) {
	svr, jwtKey := newTestServerForCV()
	repo := newFakeDeveloperCVRepo()
	repo.addProfile(developer.Developer{ID: "dev-1", Email: "dev@example.com", Slug: "dev-1", Name: "Dev"})
	repo.addProfile(developer.Developer{ID: "dev-2", Email: "other@example.com", Slug: "dev-2", Name: "Other"})
	handler := UploadDeveloperCVHandler(svr, repo)

	uploadJSON := func(cookies []*http.Cookie, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/x/udp/cv", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		addCookies(req, cookies)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	ownerCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "user-1", Email: "dev@example.com", IsDeveloper: true})
	adminCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "admin-1", Email: "admin@example.com", IsAdmin: true})
	otherCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "user-2", Email: "other@example.com", IsDeveloper: true})

	encoded := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(testPDF())
	rec := uploadJSON(ownerCookies, fmt.Sprintf(`{"id":"dev-1","cv":%q}`, encoded))
	if rec.Code != http.StatusOK {
		t.Fatalf("owner upload: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !repo.profiles["dev-1"].HasCV {
		t.Fatal("owner upload: CV was not stored")
	}

	// Admin may upload on behalf of the profile.
	rec = uploadJSON(adminCookies, fmt.Sprintf(`{"id":"dev-2","cv":%q}`, encoded))
	if rec.Code != http.StatusOK {
		t.Fatalf("admin upload: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}

	// A different developer may not upload to someone else's profile.
	rec = uploadJSON(otherCookies, fmt.Sprintf(`{"id":"dev-1","cv":%q}`, encoded))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-owner upload: expected 403, got %d", rec.Code)
	}

	// Unauthenticated requests are redirected to /auth by the middleware.
	rec = uploadJSON(nil, fmt.Sprintf(`{"id":"dev-1","cv":%q}`, encoded))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous upload: expected 401 redirect, got %d", rec.Code)
	}

	// Unknown profile.
	rec = uploadJSON(ownerCookies, fmt.Sprintf(`{"id":"no-such-profile","cv":%q}`, encoded))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown profile upload: expected 404, got %d", rec.Code)
	}

	// Invalid PDF is rejected.
	rec = uploadJSON(ownerCookies, `{"id":"dev-1","cv":"data:application/pdf;base64,`+base64.StdEncoding.EncodeToString([]byte("not a pdf"))+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-PDF upload: expected 400, got %d", rec.Code)
	}

	// Missing CV is rejected.
	rec = uploadJSON(ownerCookies, `{"id":"dev-1"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing CV upload: expected 400, got %d", rec.Code)
	}

	// Oversized JSON bodies are capped instead of buffered unboundedly.
	huge := strings.Repeat("A", maxDeveloperCVJSONRequestSize+1024)
	rec = uploadJSON(ownerCookies, fmt.Sprintf(`{"id":"dev-1","cv":%q}`, huge))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized JSON upload: expected 400, got %d", rec.Code)
	}
}

func TestUploadDeveloperCVHandlerMultipart(t *testing.T) {
	svr, jwtKey := newTestServerForCV()
	repo := newFakeDeveloperCVRepo()
	repo.addProfile(developer.Developer{ID: "dev-1", Email: "dev@example.com", Slug: "dev-1", Name: "Dev"})
	handler := UploadDeveloperCVHandler(svr, repo)
	ownerCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "user-1", Email: "dev@example.com", IsDeveloper: true})

	uploadMultipart := func(filename string, content []byte) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		fw, err := w.CreateFormFile("cv", filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteField("id", "dev-1"); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/x/udp/cv", &buf)
		req.Header.Set("Content-Type", w.FormDataContentType())
		addCookies(req, ownerCookies)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if rec := uploadMultipart("cv.pdf", testPDF()); rec.Code != http.StatusOK {
		t.Fatalf("multipart upload: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if !repo.profiles["dev-1"].HasCV {
		t.Fatal("multipart upload: CV was not stored")
	}
	if rec := uploadMultipart("cv.txt", []byte("plain text, not a pdf")); rec.Code != http.StatusBadRequest {
		t.Fatalf("multipart non-PDF upload: expected 400, got %d", rec.Code)
	}
}

func TestDownloadDeveloperCVHandler(t *testing.T) {
	svr, jwtKey := newTestServerForCV()
	repo := newFakeDeveloperCVRepo()
	repo.addProfile(developer.Developer{ID: "dev-1", Email: "dev@example.com", Slug: "dev-one", Name: "Dev", CV: testPDF()})
	repo.addProfile(developer.Developer{ID: "dev-2", Email: "nocv@example.com", Slug: "dev-two", Name: "NoCV"})
	router := mux.NewRouter()
	router.Handle("/developer/{slug}/cv", DownloadDeveloperCVHandler(svr, repo))

	download := func(cookies []*http.Cookie, url string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, url, nil)
		addCookies(req, cookies)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	recruiterCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "rec-1", Email: "rec@example.com", IsRecruiter: true})
	ownerCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "user-1", Email: "dev@example.com", IsDeveloper: true})
	otherCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "user-2", Email: "stranger@example.com", IsDeveloper: true})

	// Recruiter: inline view, audit row recorded.
	rec := download(recruiterCookies, "/developer/dev-one/cv")
	if rec.Code != http.StatusOK {
		t.Fatalf("recruiter download: expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Fatalf("recruiter download: Content-Type = %q, want application/pdf", ct)
	}
	if disp := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(disp, "inline;") {
		t.Fatalf("recruiter download: Content-Disposition = %q, want inline", disp)
	}
	if !bytes.Equal(rec.Body.Bytes(), testPDF()) {
		t.Fatal("recruiter download: body is not the stored PDF")
	}
	if len(repo.downloads) != 1 || repo.downloads[0].UserID != "rec-1" || repo.downloads[0].DeveloperProfileID != "dev-1" {
		t.Fatalf("recruiter download: expected 1 audit row for rec-1/dev-1, got %+v", repo.downloads)
	}
	if !bytes.Equal(repo.downloads[0].CV, testPDF()) {
		t.Fatal("recruiter download: audit row must store the downloaded CV bytes")
	}

	// ?download=1 serves an attachment instead of inline.
	rec = download(recruiterCookies, "/developer/dev-one/cv?download=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("attachment download: expected 200, got %d", rec.Code)
	}
	if disp := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(disp, "attachment;") {
		t.Fatalf("attachment download: Content-Disposition = %q, want attachment", disp)
	}

	// Owner may download their own CV.
	rec = download(ownerCookies, "/developer/dev-one/cv")
	if rec.Code != http.StatusOK {
		t.Fatalf("owner download: expected 200, got %d", rec.Code)
	}

	// Another developer may not.
	rec = download(otherCookies, "/developer/dev-one/cv")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-owner download: expected 403, got %d", rec.Code)
	}

	// Anonymous users are redirected to /auth.
	rec = download(nil, "/developer/dev-one/cv")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous download: expected 401 redirect, got %d", rec.Code)
	}

	// Profile without a CV: 404 with "CV not uploaded".
	rec = download(recruiterCookies, "/developer/dev-two/cv")
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "CV not uploaded") {
		t.Fatalf("no-CV download: expected 404 'CV not uploaded', got %d (%s)", rec.Code, rec.Body.String())
	}
	if len(repo.downloads) != 3 {
		t.Fatalf("no-CV download must not create an audit row, downloads = %d", len(repo.downloads))
	}

	// Unknown slug: 404, no audit row.
	rec = download(recruiterCookies, "/developer/no-such-slug/cv")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown slug download: expected 404, got %d", rec.Code)
	}
}

func TestDownloadDeveloperCVHandlerAuditFailureFailsClosed(t *testing.T) {
	svr, jwtKey := newTestServerForCV()
	repo := newFakeDeveloperCVRepo()
	repo.addProfile(developer.Developer{ID: "dev-1", Email: "dev@example.com", Slug: "dev-one", Name: "Dev", CV: testPDF()})
	repo.failAudit = true
	router := mux.NewRouter()
	router.Handle("/developer/{slug}/cv", DownloadDeveloperCVHandler(svr, repo))
	recruiterCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "rec-1", Email: "rec@example.com", IsRecruiter: true})

	req := httptest.NewRequest(http.MethodGet, "/developer/dev-one/cv", nil)
	addCookies(req, recruiterCookies)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("audit failure: expected 500, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatal("audit failure: no PDF bytes may be served when the download cannot be audited")
	}
}

func TestDeleteDeveloperCVHandler(t *testing.T) {
	svr, jwtKey := newTestServerForCV()
	repo := newFakeDeveloperCVRepo()
	repo.addProfile(developer.Developer{ID: "dev-1", Email: "dev@example.com", Slug: "dev-one", Name: "Dev", CV: testPDF()})
	handler := DeleteDeveloperCVHandler(svr, repo)
	ownerCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "user-1", Email: "dev@example.com", IsDeveloper: true})
	otherCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "user-2", Email: "stranger@example.com", IsDeveloper: true})

	deleteReq := func(cookies []*http.Cookie, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/x/udp/cv", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		addCookies(req, cookies)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	// Non-owner cannot delete.
	if rec := deleteReq(otherCookies, `{"id":"dev-1"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("non-owner delete: expected 403, got %d", rec.Code)
	}
	if !repo.profiles["dev-1"].HasCV {
		t.Fatal("non-owner delete must not remove the CV")
	}

	// Owner deletes: CV gone, subsequent download would 404.
	if rec := deleteReq(ownerCookies, `{"id":"dev-1"}`); rec.Code != http.StatusOK {
		t.Fatalf("owner delete: expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	if repo.profiles["dev-1"].HasCV || len(repo.profiles["dev-1"].CV) != 0 {
		t.Fatal("owner delete: CV was not removed")
	}
}

func TestListDeveloperCVDownloadsHandler(t *testing.T) {
	svr, jwtKey := newTestServerForCV()
	repo := newFakeDeveloperCVRepo()
	repo.addProfile(developer.Developer{ID: "dev-1", Email: "dev@example.com", Slug: "dev-one", Name: "Dev", CV: testPDF()})
	repo.downloads = append(repo.downloads,
		developer.DeveloperCVDownload{ID: "dl-1", DeveloperProfileID: "dev-1", CV: testPDF(), UserID: "rec-1", DownloadedAt: time.Now().UTC()},
		developer.DeveloperCVDownload{ID: "dl-2", DeveloperProfileID: "dev-1", CV: testPDF(), UserID: "rec-2", DownloadedAt: time.Now().UTC()},
	)
	handler := ListDeveloperCVDownloadsHandler(svr, repo)
	ownerCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "user-1", Email: "dev@example.com", IsDeveloper: true})
	adminCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "admin-1", Email: "admin@example.com", IsAdmin: true})
	otherCookies := attachUserSession(t, svr, jwtKey, middleware.UserJWT{UserID: "user-2", Email: "stranger@example.com", IsDeveloper: true})

	list := func(cookies []*http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/x/udp/cv/downloads?id=dev-1", nil)
		addCookies(req, cookies)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	rec := list(ownerCookies)
	if rec.Code != http.StatusOK {
		t.Fatalf("owner list: expected 200, got %d", rec.Code)
	}
	var body struct {
		DeveloperProfileID string `json:"developer_profile_id"`
		DownloadCount      int    `json:"download_count"`
		Downloads          []struct {
			ID     string `json:"id"`
			UserID string `json:"user_id"`
		} `json:"downloads"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("owner list: unable to decode response: %v", err)
	}
	if body.DownloadCount != 2 || len(body.Downloads) != 2 {
		t.Fatalf("owner list: expected count 2 and 2 records, got %+v", body)
	}
	// The listing must never include CV bytes.
	if strings.Contains(rec.Body.String(), "%PDF") {
		t.Fatal("owner list: response must not contain CV bytes")
	}

	// Admin may list as well.
	if rec := list(adminCookies); rec.Code != http.StatusOK {
		t.Fatalf("admin list: expected 200, got %d", rec.Code)
	}
	// Another developer may not.
	if rec := list(otherCookies); rec.Code != http.StatusForbidden {
		t.Fatalf("non-owner list: expected 403, got %d", rec.Code)
	}
	// Anonymous users are redirected to /auth.
	if rec := list(nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous list: expected 401 redirect, got %d", rec.Code)
	}
}
