package handler

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestDecodeDeveloperCV(t *testing.T) {
	validPDF := []byte("%PDF-1.4\n")
	encodedPDF := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(validPDF)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "empty cv is optional", input: "", wantErr: false},
		{name: "valid pdf data url", input: encodedPDF, wantErr: false},
		{name: "non pdf data url", input: "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("hello")), wantErr: true},
		{name: "invalid base64", input: "data:application/pdf;base64,not-base64", wantErr: true},
		{name: "too large", input: base64.StdEncoding.EncodeToString(append([]byte("%PDF"), []byte(strings.Repeat("a", maxDeveloperCVSize))...)), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeDeveloperCV(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeDeveloperCV() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
