package redis

import (
	"errors"
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
