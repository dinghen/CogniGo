package aihelper

import (
	"errors"
	"fmt"
	"testing"

	"github.com/dinghen/CogniGo/common/rag"
)

func TestRAGQueryFailureOnlyFallsBackWhenUserHasNoKnowledgeFiles(t *testing.T) {
	if !shouldFallbackToPlainChat(fmt.Errorf("list user documents: %w", rag.ErrNoKnowledgeFiles)) {
		t.Fatal("missing knowledge files should preserve ordinary chat")
	}
	if shouldFallbackToPlainChat(fmt.Errorf("query vector index: %w", rag.ErrIndexRebuildRequired)) {
		t.Fatal("index rebuild errors must not fall back to ungrounded generation")
	}
	if shouldFallbackToPlainChat(errors.New("embedding provider unavailable")) {
		t.Fatal("provider failures must not fall back to ungrounded generation")
	}
}
