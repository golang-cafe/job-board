package handler

import (
	"encoding/base64"
	"testing"
)

func TestValidateDeveloperCVBytes(t *testing.T) {
	validPDF := []byte("%PDF-1.7
1 0 obj
<<>>
endobj
")
	if err := ValidateDeveloperCVBytes(validPDF); err != nil {
		t.Fatalf("expected valid PDF bytes to pass validation, got: %v", err)
	}
	if err := ValidateDeveloperCVBytes(nil); err == nil {
		t.Fatalf("expected empty CV bytes to fail validation")
	}
	if err := ValidateDeveloperCVBytes([]byte("not a pdf")); err == nil {
		t.Fatalf("expected non-PDF bytes to fail validation")
	}
}

func TestDecodeBase64CVAndStatus(t *testing.T) {
	raw := []byte("%PDF-1.4 sample cv")
	encoded := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(raw)
	decoded, err := DecodeBase64CV(encoded)
	if err != nil {
		t.Fatalf("unexpected error decoding data URI base64 CV: %v", err)
	}
	if string(decoded) != string(raw) {
		t.Fatalf("decoded CV mismatch: %s", string(decoded))
	}
	if got := FormatDeveloperCVStatus(nil); got != "CV not uploaded" {
		t.Fatalf("expected 'CV not uploaded', got %s", got)
	}
	if got := FormatDeveloperCVStatus(decoded); got != "CV uploaded" {
		t.Fatalf("expected 'CV uploaded', got %s", got)
	}
}
