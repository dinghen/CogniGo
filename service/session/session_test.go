package session

import (
	"errors"
	"testing"

	"github.com/dinghen/CogniGo/common/code"
	"github.com/dinghen/CogniGo/common/rag"
	"github.com/dinghen/CogniGo/model"
)

func TestNormalizeModelTypeUsesUnifiedRuntimeOnlyForNewRequests(t *testing.T) {
	if got := normalizeModelType("", CapabilitySelection{}); got != "capability" {
		t.Fatalf("empty model type = %q, want capability", got)
	}
	if got := normalizeModelType("2", CapabilitySelection{}); got != "2" {
		t.Fatalf("legacy model type = %q, want 2", got)
	}
}

func TestDecodeMCPSelectionHandlesEmptyAndStoredSelections(t *testing.T) {
	servers, tools := decodeMCPSelection(&model.Session{MCPServerIDsJSON: `[]`, MCPToolIDsJSON: `[]`})
	if len(servers) != 0 || len(tools) != 0 {
		t.Fatalf("empty selections should decode empty: %v %v", servers, tools)
	}
	servers, tools = decodeMCPSelection(&model.Session{MCPServerIDsJSON: `[3,4]`, MCPToolIDsJSON: `[7]`})
	if len(servers) != 2 || servers[0] != 3 || servers[1] != 4 || len(tools) != 1 || tools[0] != 7 {
		t.Fatalf("stored selections decoded incorrectly: %v %v", servers, tools)
	}
}

func TestModelRuntimeErrorCodePreservesActionableRAGFailure(t *testing.T) {
	err := errors.Join(errors.New("prepare knowledge retrieval"), rag.ErrIndexRebuildRequired)
	if got := modelRuntimeErrorCode(err); got != code.AIModelRebuildRequired {
		t.Fatalf("modelRuntimeErrorCode = %v, want %v", got, code.AIModelRebuildRequired)
	}
	if got := modelRuntimeErrorCode(errors.New("upstream unavailable")); got != code.AIModelFail {
		t.Fatalf("modelRuntimeErrorCode for generic failure = %v, want %v", got, code.AIModelFail)
	}
}
