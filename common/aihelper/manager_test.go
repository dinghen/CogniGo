package aihelper

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
	providerService "github.com/dinghen/CogniGo/service/provider"
)

type helperTestModel struct{ modelType string }

func (m helperTestModel) GenerateResponse(context.Context, []*schema.Message) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: "ok"}, nil
}

func (m helperTestModel) StreamResponse(context.Context, []*schema.Message, StreamCallback) (string, error) {
	return "ok", nil
}

func (m helperTestModel) GetModelType() string { return m.modelType }

func TestCapabilityFingerprintChangesWithProviderSecretWithoutExposingIt(t *testing.T) {
	config := map[string]interface{}{
		"chatProvider": providerService.ResolvedConfig{Name: "chat", Kind: "chat", BaseURL: "https://example.test/v1", Model: "model-a", APIKey: "secret-one"},
	}
	first := capabilityFingerprint("capability", config)
	config["chatProvider"] = providerService.ResolvedConfig{Name: "chat", Kind: "chat", BaseURL: "https://example.test/v1", Model: "model-a", APIKey: "secret-two"}
	second := capabilityFingerprint("capability", config)
	if first == second {
		t.Fatal("provider secret changes must invalidate the cached model")
	}
	if strings.Contains(first, "secret-one") || strings.Contains(second, "secret-two") {
		t.Fatal("provider secrets must not appear in cached model fingerprints")
	}
}

func TestReplaceModelPreservesConversationMessages(t *testing.T) {
	helper := NewAIHelper(helperTestModel{modelType: "1"}, "session-1")
	helper.AddMessage("question", "alice", true, false)
	helper.replaceModel(helperTestModel{modelType: "capability"}, "new-config")

	if got := helper.GetModelType(); got != "capability" {
		t.Fatalf("model type = %q, want capability", got)
	}
	messages := helper.GetMessages()
	if len(messages) != 1 || messages[0].Content != "question" || !messages[0].IsUser {
		t.Fatalf("conversation changed when model was replaced: %#v", messages)
	}
}
