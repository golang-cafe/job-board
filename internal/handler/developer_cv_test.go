package handler

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateOptionalLinkedinURL(t *testing.T) {
	if !ValidateOptionalLinkedinURL("") {
		t.Fatalf("expected empty LinkedIn URL to be valid (optional)")
	}
	if !ValidateOptionalLinkedinURL("https://www.linkedin.com/in/johndoe") {
		t.Fatalf("expected valid www.linkedin.com URL to pass")
	}
	if !ValidateOptionalLinkedinURL("https://linkedin.com/in/johndoe") {
		t.Fatalf("expected valid linkedin.com URL to pass")
	}
	if !ValidateOptionalLinkedinURL("https://br.linkedin.com/in/johndoe") {
		t.Fatalf("expected country-prefix linkedin URL to pass")
	}
	if ValidateOptionalLinkedinURL("https://github.com/johndoe") {
		t.Fatalf("expected non-linkedin URL to fail")
	}
	if ValidateOptionalLinkedinURL("not-a-url") {
		t.Fatalf("expected garbage URL to fail")
	}
}

func TestValidateDeveloperCVBytes(t *testing.T) {
	if _, status := ValidateDeveloperCVBytes(nil, MaxDeveloperCVSize); status != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty CV, got %d", status)
	}

	if _, status := ValidateDeveloperCVBytes([]byte("not a pdf file"), MaxDeveloperCVSize); status != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for non-PDF CV, got %d", status)
	}

	validPDF := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("A"), 512)...)
	if errMsg, status := ValidateDeveloperCVBytes(validPDF, MaxDeveloperCVSize); status != http.StatusOK {
		t.Fatalf("expected 200 for valid PDF header, got %d (%s)", status, errMsg)
	}

	tooBig := bytes.Repeat([]byte("A"), int(MaxDeveloperCVSize)+1)
	tooBig[0], tooBig[1], tooBig[2], tooBig[3] = '%', 'P', 'D', 'F'
	if _, status := ValidateDeveloperCVBytes(tooBig, MaxDeveloperCVSize); status != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized CV, got %d", status)
	}
}

func TestValidateDeveloperCVBytes_SampleFile(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve test file path")
	}
	sample := filepath.Join(filepath.Dir(thisFile), "..", "..", "testdata", "sample-cv.pdf")
	b, err := ioutil.ReadFile(sample)
	if err != nil {
		t.Fatalf("unable to read sample CV: %v", err)
	}
	if errMsg, status := ValidateDeveloperCVBytes(b, MaxDeveloperCVSize); status != http.StatusOK {
		t.Fatalf("expected sample-cv.pdf to validate, got %d (%s)", status, errMsg)
	}
}
