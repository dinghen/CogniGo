package provider

import (
	"errors"
	"strings"
	"testing"

	"github.com/dinghen/CogniGo/common/code"
	providerService "github.com/dinghen/CogniGo/service/provider"
	"gorm.io/gorm"
)

func TestSafeProviderErrorDoesNotExposeUpstreamDetails(t *testing.T) {
	secret := "upstream body contains sk-live-secret"
	status, responseCode, message := safeProviderError(errors.New(secret))
	if status != 503 || responseCode != code.CodeServerBusy || message != "provider service unavailable" {
		t.Fatalf("safeProviderError() = (%d, %d, %q)", status, responseCode, message)
	}
	if strings.Contains(message, secret) || strings.Contains(message, "sk-live-secret") {
		t.Fatalf("safe message exposed provider details: %q", message)
	}
}

func TestSafeProviderErrorClassifiesKnownErrors(t *testing.T) {
	if status, responseCode, _ := safeProviderError(gorm.ErrRecordNotFound); status != 404 || responseCode != code.CodeRecordNotFound {
		t.Fatalf("record not found classification = (%d, %d)", status, responseCode)
	}
	if status, responseCode, _ := safeProviderError(providerService.ErrInvalidKind); status != 400 || responseCode != code.CodeInvalidParams {
		t.Fatalf("invalid kind classification = (%d, %d)", status, responseCode)
	}
}
