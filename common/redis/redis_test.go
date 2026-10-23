package redis

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	redisCli "github.com/redis/go-redis/v9"
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
	if err := DeleteCaptchaForEmail("user@example.com", "123456"); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("DeleteCaptchaForEmail error = %v, want ErrNotInitialized", err)
	}
	if err := RestoreCaptchaForEmail("user@example.com", "123456"); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("RestoreCaptchaForEmail error = %v, want ErrNotInitialized", err)
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

func TestCaptchaConsumptionIsAtomicIntegration(t *testing.T) {
	if os.Getenv("COGNIGO_REDIS_INTEGRATION") != "1" {
		t.Skip("set COGNIGO_REDIS_INTEGRATION=1 to test captcha Redis operations")
	}
	addr := os.Getenv("COGNIGO_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	client := redisCli.NewClient(&redisCli.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis unavailable: %v", err)
	}
	previous := Rdb
	Rdb = client
	email := "cognigo-redis-test@example.invalid"
	t.Cleanup(func() {
		_ = client.Del(context.Background(), GenerateCaptcha(email)).Err()
		Rdb = previous
		_ = client.Close()
	})

	if err := SetCaptchaForEmail(email, "123456"); err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			valid, err := CheckCaptchaForEmail(email, "123456")
			if err != nil {
				t.Errorf("CheckCaptchaForEmail error = %v", err)
				return
			}
			if valid {
				accepted.Add(1)
			}
		}()
	}
	group.Wait()
	if got := accepted.Load(); got != 1 {
		t.Fatalf("concurrent captcha acceptances = %d, want 1", got)
	}

	if err := RestoreCaptchaForEmail(email, "retry1"); err != nil {
		t.Fatal(err)
	}
	if err := SetCaptchaForEmail(email, "newer1"); err != nil {
		t.Fatal(err)
	}
	if err := RestoreCaptchaForEmail(email, "stale1"); err != nil {
		t.Fatal(err)
	}
	if valid, err := CheckCaptchaForEmail(email, "stale1"); err != nil || valid {
		t.Fatalf("stale captcha check = (%t, %v), want false", valid, err)
	}
	if valid, err := CheckCaptchaForEmail(email, "newer1"); err != nil || !valid {
		t.Fatalf("newer captcha check = (%t, %v), want true", valid, err)
	}
	t.Logf("captcha integration completed within TTL %s", 2*time.Minute)
}

func TestActivateRAGIndexesValidatesClientAndInput(t *testing.T) {
	previous := Rdb
	Rdb = nil
	t.Cleanup(func() { Rdb = previous })
	if _, err := ActivateRAGIndexes(t.Context(), "alice", nil); err == nil {
		t.Fatal("ActivateRAGIndexes should reject an empty generation set")
	}
	ref, err := NewRAGIndexRef("alice", "doc.md", "https://embed.example/v1", "model-a", 4, "build-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ActivateRAGIndexes(t.Context(), "alice", map[string]RAGIndexRef{"doc.md": ref}); !errors.Is(err, ErrNotInitialized) {
		t.Fatalf("ActivateRAGIndexes error = %v, want ErrNotInitialized", err)
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
