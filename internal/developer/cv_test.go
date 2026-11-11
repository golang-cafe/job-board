package developer

import (
	"bytes"
	"testing"
)

func TestValidateCVBytes(t *testing.T) {
	if err := ValidateCVBytes(nil); err != ErrCVEmpty {
		t.Fatalf("expected ErrCVEmpty, got %v", err)
	}
	if err := ValidateCVBytes([]byte("not a pdf")); err != ErrCVInvalidFormat {
		t.Fatalf("expected ErrCVInvalidFormat, got %v", err)
	}
	validPDF := append([]byte("%PDF-1.7
"), bytes.Repeat([]byte("A"), 256)...)
	if err := ValidateCVBytes(validPDF); err != nil {
		t.Fatalf("expected nil for valid PDF, got %v", err)
	}
}

func TestFormatCVStatusLabel(t *testing.T) {
	if got := FormatCVStatusLabel(nil); got != "CV not uploaded" {
		t.Fatalf("expected 'CV not uploaded', got %q", got)
	}
}

func TestAuthorizeCVDownload(t *testing.T) {
	if err := AuthorizeCVDownload(false, true, false); err != ErrCVUnauthorized {
		t.Fatalf("unauthenticated user must be rejected")
	}
	if err := AuthorizeCVDownload(true, false, false); err != ErrCVUnauthorized {
		t.Fatalf("non-recruiter/non-admin user must be rejected")
	}
	if err := AuthorizeCVDownload(true, true, false); err != nil {
		t.Fatalf("logged-in recruiter should be allowed, got %v", err)
	}
	if err := AuthorizeCVDownload(true, false, true); err != nil {
		t.Fatalf("logged-in admin should be allowed, got %v", err)
	}
}
