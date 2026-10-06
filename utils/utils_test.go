package utils

import (
	"mime/multipart"
	"testing"
)

func TestValidateFileRejectsNilHeader(t *testing.T) {
	if err := ValidateFile(nil); err == nil {
		t.Fatal("ValidateFile(nil) should return an error")
	}
}

func TestValidateFileAllowsMarkdownAndText(t *testing.T) {
	for _, filename := range []string{"notes.md", "notes.TXT"} {
		if err := ValidateFile(&multipart.FileHeader{Filename: filename}); err != nil {
			t.Fatalf("ValidateFile(%q) returned error: %v", filename, err)
		}
	}
}
