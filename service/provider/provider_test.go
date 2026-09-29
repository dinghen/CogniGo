package provider

import (
	"strings"
	"testing"

	"github.com/dinghen/CogniGo/common/mysql"
)

func TestEncryptDecryptAndMaskSecret(t *testing.T) {
	t.Setenv("COGNIGO_PROVIDER_ENCRYPTION_KEY", "01234567890123456789012345678901")
	encoded, err := EncryptSecret("sk-user-secret-1234")
	if err != nil {
		t.Fatalf("EncryptSecret() error = %v", err)
	}
	if encoded == "" || strings.Contains(encoded, "sk-user-secret") {
		t.Fatalf("encrypted value contains plaintext: %q", encoded)
	}
	decoded, err := DecryptSecret(encoded)
	if err != nil || decoded != "sk-user-secret-1234" {
		t.Fatalf("DecryptSecret() = %q, %v", decoded, err)
	}
	if got := MaskSecret("sk-user-secret-1234"); got != "sk-u...1234" {
		t.Fatalf("MaskSecret() = %q", got)
	}
}

func TestEncryptSecretRequiresValidMasterKey(t *testing.T) {
	t.Setenv("COGNIGO_PROVIDER_ENCRYPTION_KEY", "too-short")
	if _, err := EncryptSecret("secret"); err == nil {
		t.Fatal("EncryptSecret() should reject a short master key")
	}
}

func TestValidateInput(t *testing.T) {
	valid := Input{Name: "dashscope", Kind: "embedding", BaseURL: "https://example.test/v1", Model: "embed"}
	if err := validateInput(valid); err != nil {
		t.Fatalf("validateInput(valid) error = %v", err)
	}
	valid.Kind = "admin"
	if err := validateInput(valid); err != ErrInvalidKind {
		t.Fatalf("validateInput(invalid kind) = %v", err)
	}
	valid.Kind = "chat"
	valid.Name = ""
	if err := validateInput(valid); err != ErrInvalidInput {
		t.Fatalf("validateInput(invalid fields) = %v", err)
	}
}

func TestResolveDoesNotHideDatabaseFailure(t *testing.T) {
	previous := mysql.DB
	mysql.DB = nil
	t.Cleanup(func() { mysql.DB = previous })
	if _, err := Resolve("demo", "chat"); err == nil || !strings.Contains(err.Error(), "database is not initialized") {
		t.Fatalf("Resolve() error = %v, want explicit database initialization error", err)
	}
}
