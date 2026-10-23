package code

import (
	"strings"
	"testing"
)

func TestAIModelFailMessageIsActionableAndDoesNotExposeUpstreamDetails(t *testing.T) {
	message := AIModelFail.Msg()
	if !strings.Contains(message, "配置") || !strings.Contains(message, "Embedding") || !strings.Contains(message, "MCP") {
		t.Fatalf("AIModelFail message is not actionable: %q", message)
	}
	if strings.Contains(message, "503") || strings.Contains(message, "distributor") {
		t.Fatalf("AIModelFail message must not expose upstream details: %q", message)
	}
}
