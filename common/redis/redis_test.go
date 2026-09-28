package redis

import (
	"errors"
	"strings"
	"testing"
)

func TestRedisOperationsFailClearlyBeforeInitialization(t *testing.T) {
	previous := Rdb
	Rdb = nil
	t.Cleanup(func() { Rdb = previous })

	if err := SetCaptchaForEmail("user@example.com", "123456"); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("SetCaptchaForEmail error = %v, want ErrNotInitialized", err)
	}
	if _, err := CheckCaptchaForEmail("user@example.com", "123456"); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("CheckCaptchaForEmail error = %v, want ErrNotInitialized", err)
	}
	if err := InitRedisIndex(t.Context(), "alice", "doc.md", 4); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("InitRedisIndex error = %v, want ErrNotInitialized", err)
	}
	if err := DeleteRedisIndex(t.Context(), "alice", "doc.md"); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("DeleteRedisIndex error = %v, want ErrNotInitialized", err)
	}
}

func TestUnknownIndexErrorsAreRecognizedCaseInsensitively(t *testing.T) {
	if !isUnknownIndexError(errors.New("Unknown Index name: rag_docs")) {
		t.Fatal("expected unknown index error to be recognized")
	}
	if isUnknownIndexError(errors.New("connection refused")) {
		t.Fatal("connection errors must not be treated as missing indexes")
	}
}

func TestRAGIndexRefIdentityAndValidation(t *testing.T) {
	first, err := NewRAGIndexRef("alice", "doc.md", "https://embed.example/v1", "model-a", 4, "build-1")
	if err != nil {
		t.Fatalf("NewRAGIndexRef error = %v", err)
	}
	second, err := NewRAGIndexRef("alice", "doc.md", "https://embed.example/v1", "model-a", 4, "build-2")
	if err != nil {
		t.Fatalf("NewRAGIndexRef error = %v", err)
	}
	if first.Fingerprint != second.Fingerprint {
		t.Fatal("same embedding identity should produce a stable fingerprint")
	}
	if first.IndexName == second.IndexName || first.KeyPrefix == second.KeyPrefix {
		t.Fatal("index generations must have isolated names and key prefixes")
	}
	if !first.MatchesEmbedding(second.Endpoint, second.Model, second.Dimension) {
		t.Fatal("same endpoint, model, and dimension should match")
	}

	first.Fingerprint = "incorrect"
	if err := first.Validate(); err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("Validate error = %v, want fingerprint mismatch", err)
	}
}

func TestFindRESPPairReadsNestedVectorDimension(t *testing.T) {
	info := []interface{}{
		"index_name", "rag_idx",
		"attributes", []interface{}{
			[]interface{}{"identifier", "vector", "attribute", "vector", "type", "FLOAT32", "dim", int64(768)},
		},
	}
	value, ok := findRESPPair(info, "dim")
	if !ok || value != int64(768) {
		t.Fatalf("findRESPPair(dim) = (%v, %v), want (768, true)", value, ok)
	}
	if _, ok := findRESPPair(info, "missing"); ok {
		t.Fatal("findRESPPair should report missing keys")
	}
	mapInfo := map[string]interface{}{"attributes": []interface{}{map[string]interface{}{"DIM": "1024"}}}
	value, ok = findRESPPair(mapInfo, "dim")
	if !ok || value != "1024" {
		t.Fatalf("findRESPPair on RESP3-style map = (%v, %v), want (1024, true)", value, ok)
	}
}
