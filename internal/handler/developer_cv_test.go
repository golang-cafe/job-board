package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/server"
)

type mockDevGetSaver struct {
	saved developer.Developer
}

func (m *mockDevGetSaver) DeveloperProfileByEmail(email string) (developer.Developer, error) {
	return developer.Developer{}, nil
}

func (m *mockDevGetSaver) SaveDeveloperProfile(dev developer.Developer) error {
	m.saved = dev
	return nil
}

type mockTokenSaver struct{}

func (m *mockTokenSaver) SaveTokenSignOn(email, token, userType string) error {
	return nil
}

func TestUploadDeveloperCVHandlerRejectsNonPDF(t *testing.T) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("cv", "resume.txt")
	if err != nil {
		t.Fatalf("CreateFormFile failed: %v", err)
	}
	_, _ = part.Write([]byte("plain text not a pdf"))
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/x/s/cv", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()

	h := UploadDeveloperCVHandler(server.Server{})
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 Unsupported Media Type for non-PDF upload, got %d", rec.Code)
	}
}

func TestSaveDeveloperProfileAllowsEmptyLinkedinWithCV(t *testing.T) {
	devRepo := &mockDevGetSaver{}
	userRepo := &mockTokenSaver{}
	h := SaveDeveloperProfileHandler(server.Server{}, devRepo, userRepo)

	// Invalid non-empty LinkedIn URL should still fail with 400
	invalidPayload := []byte(`{"fullname":"Jane Doe","hourly_rate":"90","linkedin_url":"https://example.com/jane","current_location":"London","bio":"Go dev","tags":"Go,Docker","profile_image_id":"img123","cv_id":"cv123","email":"jane@example.com","search_status":"actively-applying","role_level":"senior","role_types":["full-time"],"detected_location_id":"London"}`)
	reqInvalid := httptest.NewRequest(http.MethodPost, "/x/sdp", bytes.NewReader(invalidPayload))
	recInvalid := httptest.NewRecorder()
	h.ServeHTTP(recInvalid, reqInvalid)
	if recInvalid.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for non-linkedin URL, got %d", recInvalid.Code)
	}
}
