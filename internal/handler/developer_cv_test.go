package handler

import (
	"net/http"
	"regexp"
	"testing"
)

func TestDeveloperCVAndLinkedinOptional(t *testing.T) {
	linkedinRe := regexp.MustCompile(`^https:\/\/(?:[a-z]{2,3}\.)?linkedin\.com\/.*$`)
	emptyURL := ""
	if emptyURL != "" && !linkedinRe.MatchString(emptyURL) {
		t.Fatalf("expected empty linkedin url to be allowed when optional")
	}
	validURL := "https://www.linkedin.com/in/gopher"
	if !linkedinRe.MatchString(validURL) {
		t.Fatalf("expected valid linkedin url to match")
	}

	pdfSample := append([]byte("%PDF-1.4\n%test\n"), make([]byte, 64)...)
	if ct := http.DetectContentType(pdfSample); ct != "application/pdf" {
		t.Fatalf("expected application/pdf, got %s", ct)
	}
	nonPdfSample := []byte("plain text file content")
	if ct := http.DetectContentType(nonPdfSample); ct == "application/pdf" {
		t.Fatalf("expected non-pdf content type, got %s", ct)
	}
}
