package rag

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/dinghen/CogniGo/common/redis"
	redisClient "github.com/redis/go-redis/v9"
)

type integrationEmbedder struct{}

func (integrationEmbedder) EmbedStrings(_ context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	result := make([][]float64, len(texts))
	for i, text := range texts {
		vector := []float64{0, 0, 0, 0}
		for _, char := range text {
			vector[int(char)%len(vector)] += 1
		}
		result[i] = vector
	}
	return result, nil
}

// Run with COGNIGO_REDIS_INTEGRATION=1 against Redis Stack. It is skipped in
// ordinary unit-test runs so contributors do not need local infrastructure.
func TestRedisStackIndexGenerationIntegration(t *testing.T) {
	if os.Getenv("COGNIGO_REDIS_INTEGRATION") != "1" {
		t.Skip("set COGNIGO_REDIS_INTEGRATION=1 to run Redis Stack integration")
	}
	addr := os.Getenv("COGNIGO_REDIS_ADDR")
	if addr == "" {
		addr = "127.0.0.1:6379"
	}
	client := redisClient.NewClient(&redisClient.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis Stack unavailable: %v", err)
	}
	redis.Rdb = client
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "guide.md")
	if err := os.WriteFile(path, []byte("# Guide\n\nRedis vector retrieval"), 0600); err != nil {
		t.Fatal(err)
	}
	indexer, err := newRAGIndexer(ctx, "integration-user", "guide.md", "https://embed.example", "fake", 4, integrationEmbedder{})
	if err != nil {
		t.Fatal(err)
	}
	if err := indexer.IndexFile(ctx, path); err != nil {
		t.Fatal(err)
	}
	active, err := redis.GetActiveRAGIndex(ctx, "integration-user", "guide.md")
	if err != nil || active == nil {
		t.Fatalf("active index = %#v, err=%v", active, err)
	}
	if err := redis.DeleteRAGIndex(ctx, "integration-user", "guide.md"); err != nil {
		t.Fatal(err)
	}
}
