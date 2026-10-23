package rag

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/dinghen/CogniGo/common/mysql"
	"github.com/dinghen/CogniGo/common/redis"
	"github.com/dinghen/CogniGo/config"
	providerService "github.com/dinghen/CogniGo/service/provider"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Run with COGNIGO_RAG_PROVIDER_INTEGRATION=1, COGNIGO_RAG_LIVE_USERNAME, and
// COGNIGO_RAG_LIVE_QUERY after loading the local demo environment.
func TestProviderBackedRAGRetrievalIntegration(t *testing.T) {
	if os.Getenv("COGNIGO_RAG_PROVIDER_INTEGRATION") != "1" {
		t.Skip("set COGNIGO_RAG_PROVIDER_INTEGRATION=1 to run provider-backed RAG retrieval")
	}
	username := os.Getenv("COGNIGO_RAG_LIVE_USERNAME")
	queryText := os.Getenv("COGNIGO_RAG_LIVE_QUERY")
	if username == "" || queryText == "" {
		t.Fatal("COGNIGO_RAG_LIVE_USERNAME and COGNIGO_RAG_LIVE_QUERY are required")
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repository root for integration config")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "../.."))
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change to repository root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(workingDirectory) })

	if err := config.InitConfig(); err != nil {
		t.Fatalf("load integration config: %v", err)
	}
	previousDB := mysql.DB
	cfg := config.GetConfig()
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=true&loc=Local", cfg.MysqlUser, cfg.MysqlPassword, cfg.MysqlHost, cfg.MysqlPort, cfg.MysqlDatabaseName, cfg.MysqlCharset)
	db, err := gorm.Open(mysqlDriver.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect to MySQL: %v", err)
	}
	mysql.DB = db
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		mysql.DB = previousDB
	})
	previousRedis := redis.Rdb
	redis.Init()
	client := redis.Rdb
	t.Cleanup(func() {
		if client != nil {
			_ = client.Close()
		}
		redis.Rdb = previousRedis
	})

	resolved, err := providerService.Resolve(username, "embedding")
	if err != nil {
		t.Fatalf("resolve embedding provider: %v", err)
	}
	query, err := NewRAGQueryForProvider(context.Background(), username, resolved)
	if err != nil {
		t.Fatalf("create RAG query: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	documents, err := query.RetrieveDocuments(ctx, queryText)
	if err != nil {
		t.Fatalf("retrieve documents: %v", err)
	}
	if len(documents) == 0 {
		t.Fatal("provider-backed retrieval returned no documents")
	}
	t.Logf("provider-backed retrieval returned %d document(s); nearest distance %.4f", len(documents), documentDistance(documents[0]))
}
