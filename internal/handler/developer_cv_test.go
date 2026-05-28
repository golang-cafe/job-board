package handler

import (
	"bytes"
	"testing"
)

func TestValidateDeveloperProfileCVAcceptsPDF(t *testing.T) {
	pdf := []byte("%PDF-1.4\n1 0 obj\n<<>>\nendobj\ntrailer\n<<>>\n%%EOF")

	if err := validateDeveloperProfileCV(pdf); err != nil {
		t.Fatalf("expected PDF CV to be valid, got %v", err)
	}
}

func TestValidateDeveloperProfileCVRejectsNonPDF(t *testing.T) {
	if err := validateDeveloperProfileCV([]byte("not a pdf")); err == nil {
		t.Fatal("expected non-PDF CV to be rejected")
	}
}

func TestValidateDeveloperProfileCVRejectsOversizedFile(t *testing.T) {
	oversizedPDF := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), developerProfileCVMaxSize)...)

	if err := validateDeveloperProfileCV(oversizedPDF); err == nil {
		t.Fatal("expected oversized CV to be rejected")
	}
}
