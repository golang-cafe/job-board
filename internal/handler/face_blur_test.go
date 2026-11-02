package handler_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-cafe/job-board/internal/handler"
	"github.com/golang-cafe/job-board/internal/server"
)

func TestSaveMediaPageHandlerRejectsDeveloperImageWithoutFace(t *testing.T) {
	blank := image.NewRGBA(image.Rect(0, 0, 120, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			blank.Set(x, y, color.RGBA{R: 100, G: 110, B: 120, A: 255})
		}
	}
	var imgBuf bytes.Buffer
	if err := png.Encode(&imgBuf, blank); err != nil {
		t.Fatalf("failed to encode png: %v", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "avatar.png")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write(imgBuf.Bytes()); err != nil {
		t.Fatalf("failed to write image bytes: %v", err)
	}
	writer.Close()

	req, err := http.NewRequest(http.MethodPost, "/x/s/m?x=0&y=0&w=100&h=100&type=developer", &body)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	rr := httptest.NewRecorder()
	svr := server.Server{}
	h := handler.SaveMediaPageHandler(svr)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d when no face detected, got %d (body: %s)", http.StatusBadRequest, rr.Code, rr.Body.String())
	}
}
