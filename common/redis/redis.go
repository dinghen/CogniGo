package redis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dinghen/CogniGo/config"
	redisCli "github.com/redis/go-redis/v9"
)

var Rdb *redisCli.Client

var ctx = context.Background()

var ErrNotInitialized = errors.New("redis client is not initialized")

// RAGIndexRef describes one fully built, model-compatible vector index.
type RAGIndexRef struct {
	Endpoint    string `json:"endpoint"`
	Model       string `json:"model"`
	Dimension   int    `json:"dimension"`
	Fingerprint string `json:"fingerprint"`
	IndexName   string `json:"index_name"`
	KeyPrefix   string `json:"key_prefix"`
	Generation  string `json:"generation"`
}

func NewRAGIndexRef(username, filename, endpoint, model string, dimension int, generation string) (RAGIndexRef, error) {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(filename) == "" {
		return RAGIndexRef{}, fmt.Errorf("username and filename are required")
	}
	if strings.TrimSpace(endpoint) == "" || strings.TrimSpace(model) == "" {
		return RAGIndexRef{}, fmt.Errorf("embedding endpoint and model are required")
	}
	if dimension <= 0 {
		return RAGIndexRef{}, fmt.Errorf("embedding dimension must be positive")
	}
	if strings.TrimSpace(generation) == "" {
		return RAGIndexRef{}, fmt.Errorf("index generation is required")
	}

	identity := fmt.Sprintf("%s\x00%s\x00%d", endpoint, model, dimension)
	fingerprintSum := sha256.Sum256([]byte(identity))
	namespaceSum := sha256.Sum256([]byte(username + "\x00" + filename))
	fingerprint := hex.EncodeToString(fingerprintSum[:])
	namespace := hex.EncodeToString(namespaceSum[:8])
	return RAGIndexRef{
		Endpoint: endpoint, Model: model, Dimension: dimension,
		Fingerprint: fingerprint,
		IndexName:   fmt.Sprintf("rag_idx_%s_%s_%s", namespace, fingerprint[:16], generation),
		KeyPrefix:   fmt.Sprintf("%sv:%s:", GenerateUserIndexNamePrefix(username, filename), generation),
		Generation:  generation,
	}, nil
}

func (ref RAGIndexRef) Validate() error {
	if strings.TrimSpace(ref.Endpoint) == "" || strings.TrimSpace(ref.Model) == "" {
		return fmt.Errorf("embedding endpoint and model are required")
	}
	if ref.Dimension <= 0 {
		return fmt.Errorf("embedding dimension must be positive")
	}
	if ref.IndexName == "" || ref.KeyPrefix == "" || ref.Generation == "" {
		return fmt.Errorf("index name, key prefix, and generation are required")
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", ref.Endpoint, ref.Model, ref.Dimension)))
	if ref.Fingerprint != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("embedding index fingerprint does not match its identity")
	}
	return nil
}

func (ref RAGIndexRef) MatchesEmbedding(endpoint, model string, dimension int) bool {
	return ref.Endpoint == endpoint && ref.Model == model && ref.Dimension == dimension
}

func client() (*redisCli.Client, error) {
	if Rdb == nil {
		return nil, ErrNotInitialized
	}
	return Rdb, nil
}

func Init() {
	conf := config.GetConfig()
	host := conf.RedisConfig.RedisHost
	port := conf.RedisConfig.RedisPort
	password := conf.RedisConfig.RedisPassword
	db := conf.RedisDb
	addr := host + ":" + strconv.Itoa(port)

	Rdb = redisCli.NewClient(&redisCli.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		Protocol: 2, // 使用 Protocol 2 避免 maint_notifications 警告
	})

}

func SetCaptchaForEmail(email, captcha string) error {
	rdb, err := client()
	if err != nil {
		return err
	}
	key := GenerateCaptcha(email)
	expire := 2 * time.Minute
	return rdb.Set(ctx, key, captcha, expire).Err()
}

func CheckCaptchaForEmail(email, userInput string) (bool, error) {
	rdb, err := client()
	if err != nil {
		return false, err
	}
	key := GenerateCaptcha(email)

	storedCaptcha, err := rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redisCli.Nil {

			return false, nil
		}

		return false, err
	}

	if strings.EqualFold(storedCaptcha, userInput) {

		// 验证成功后删除 key
		if err := rdb.Del(ctx, key).Err(); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

// InitRedisIndex 初始化 Redis 索引，支持按文件名区分
func InitRedisIndex(ctx context.Context, username, filename string, dimension int) error {
	rdb, err := client()
	if err != nil {
		return err
	}
	if strings.TrimSpace(username) == "" || strings.TrimSpace(filename) == "" {
		return fmt.Errorf("username and filename are required")
	}
	if dimension <= 0 {
		return fmt.Errorf("embedding dimension must be positive")
	}
	indexName := GenerateUserIndexName(username, filename)

	// 检查索引是否存在
	_, err = rdb.Do(ctx, "FT.INFO", indexName).Result()
	if err == nil {
		fmt.Println("索引已存在，跳过创建")
		return nil
	}

	// 如果索引不存在，创建新索引
	if !isUnknownIndexError(err) {
		return fmt.Errorf("检查索引失败: %w", err)
	}

	if err := createRAGRedisIndex(ctx, rdb, indexName, GenerateUserIndexNamePrefix(username, filename), dimension); err != nil {
		return err
	}

	fmt.Println("索引创建成功！")
	return nil
}

// CreateRAGIndex creates an isolated index generation so a rebuild can finish
// before it becomes visible to retrieval.
func CreateRAGIndex(ctx context.Context, ref RAGIndexRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	rdb, err := client()
	if err != nil {
		return err
	}
	return createRAGRedisIndex(ctx, rdb, ref.IndexName, ref.KeyPrefix, ref.Dimension)
}

// GetActiveRAGIndex returns the active index only when its Redis index exists
// and its vector schema still has the recorded dimension. A stale or
// incompatible pointer is treated as a rebuild requirement.
func GetActiveRAGIndex(ctx context.Context, username, filename string) (*RAGIndexRef, error) {
	rdb, err := client()
	if err != nil {
		return nil, err
	}
	ref, err := readActiveRAGIndex(ctx, rdb, username, filename)
	if err != nil || ref == nil {
		return ref, err
	}
	dimension, err := redisIndexDimension(ctx, rdb, ref.IndexName)
	if err != nil {
		if isUnknownIndexError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect active RAG index: %w", err)
	}
	if dimension != ref.Dimension {
		return nil, nil
	}
	return ref, nil
}

// ActivateRAGIndex switches retrieval to a fully written index generation.
// The caller can remove the returned previous generation after the switch.
func ActivateRAGIndex(ctx context.Context, username, filename string, next RAGIndexRef) (*RAGIndexRef, error) {
	if err := next.Validate(); err != nil {
		return nil, err
	}
	rdb, err := client()
	if err != nil {
		return nil, err
	}
	dimension, err := redisIndexDimension(ctx, rdb, next.IndexName)
	if err != nil {
		return nil, fmt.Errorf("verify replacement RAG index: %w", err)
	}
	if dimension != next.Dimension {
		return nil, fmt.Errorf("replacement RAG index dimension is %d, expected %d", dimension, next.Dimension)
	}
	previous, err := readActiveRAGIndex(ctx, rdb, username, filename)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("encode active RAG index: %w", err)
	}
	if err := rdb.Set(ctx, activeRAGIndexKey(username, filename), encoded, 0).Err(); err != nil {
		return nil, fmt.Errorf("activate RAG index: %w", err)
	}
	return previous, nil
}

func readActiveRAGIndex(ctx context.Context, rdb *redisCli.Client, username, filename string) (*RAGIndexRef, error) {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(filename) == "" {
		return nil, fmt.Errorf("username and filename are required")
	}
	encoded, err := rdb.Get(ctx, activeRAGIndexKey(username, filename)).Bytes()
	if err != nil {
		if errors.Is(err, redisCli.Nil) {
			return nil, nil
		}
		return nil, fmt.Errorf("read active RAG index: %w", err)
	}
	var ref RAGIndexRef
	if err := json.Unmarshal(encoded, &ref); err != nil {
		return nil, fmt.Errorf("decode active RAG index: %w", err)
	}
	if err := ref.Validate(); err != nil {
		return nil, fmt.Errorf("invalid active RAG index: %w", err)
	}
	return &ref, nil
}

func activeRAGIndexKey(username, filename string) string {
	return GenerateUserIndexNamePrefix(username, filename) + "active"
}

func createRAGRedisIndex(ctx context.Context, rdb *redisCli.Client, indexName, prefix string, dimension int) error {
	if dimension <= 0 {
		return fmt.Errorf("embedding dimension must be positive")
	}
	createArgs := []interface{}{
		"FT.CREATE", indexName,
		"ON", "HASH",
		"PREFIX", "1", prefix,
		"SCHEMA",
		"content", "TEXT",
		"metadata", "TEXT",
		"source", "TEXT",
		"title", "TEXT",
		"user", "TAG",
		"original_id", "TEXT",
		"chunk_index", "NUMERIC",
		"vector", "VECTOR", "FLAT",
		"6",
		"TYPE", "FLOAT32",
		"DIM", dimension,
		"DISTANCE_METRIC", "COSINE",
	}
	if err := rdb.Do(ctx, createArgs...).Err(); err != nil {
		return fmt.Errorf("create Redis RAG index: %w", err)
	}
	return nil
}

func redisIndexDimension(ctx context.Context, rdb *redisCli.Client, indexName string) (int, error) {
	info, err := rdb.Do(ctx, "FT.INFO", indexName).Result()
	if err != nil {
		return 0, err
	}
	value, ok := findRESPPair(info, "dim")
	if !ok {
		return 0, fmt.Errorf("Redis index %q has no vector dimension in FT.INFO", indexName)
	}
	dimension, err := strconv.Atoi(fmt.Sprint(value))
	if err != nil || dimension <= 0 {
		return 0, fmt.Errorf("Redis index %q has invalid vector dimension %v", indexName, value)
	}
	return dimension, nil
}

func findRESPPair(value any, key string) (any, bool) {
	switch items := value.(type) {
	case []interface{}:
		for i := 0; i+1 < len(items); i += 2 {
			if strings.EqualFold(fmt.Sprint(items[i]), key) {
				return items[i+1], true
			}
		}
		for _, item := range items {
			if nested, ok := findRESPPair(item, key); ok {
				return nested, true
			}
		}
	case map[string]interface{}:
		for mapKey, item := range items {
			if strings.EqualFold(mapKey, key) {
				return item, true
			}
		}
		for _, item := range items {
			if nested, ok := findRESPPair(item, key); ok {
				return nested, true
			}
		}
	case map[interface{}]interface{}:
		for mapKey, item := range items {
			if strings.EqualFold(fmt.Sprint(mapKey), key) {
				return item, true
			}
		}
		for _, item := range items {
			if nested, ok := findRESPPair(item, key); ok {
				return nested, true
			}
		}
	}
	return nil, false
}

// DeleteRAGIndexRef removes one non-active or retired index generation.
func DeleteRAGIndexRef(ctx context.Context, ref RAGIndexRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	rdb, err := client()
	if err != nil {
		return err
	}
	if err := rdb.Do(ctx, "FT.DROPINDEX", ref.IndexName, "DD").Err(); err != nil && !isUnknownIndexError(err) {
		return fmt.Errorf("delete RAG index generation: %w", err)
	}
	return nil
}

// DeleteLegacyRAGIndex removes the pre-fingerprint index after a successful
// migration. It is safe to call when the legacy index is absent.
func DeleteLegacyRAGIndex(ctx context.Context, username, filename string) error {
	rdb, err := client()
	if err != nil {
		return err
	}
	if err := rdb.Do(ctx, "FT.DROPINDEX", GenerateUserIndexName(username, filename), "DD").Err(); err != nil && !isUnknownIndexError(err) {
		return fmt.Errorf("delete legacy RAG index: %w", err)
	}
	return nil
}

// DeleteRAGIndex removes the active index, its activation pointer, and any
// legacy index created before embedding identities were persisted.
func DeleteRAGIndex(ctx context.Context, username, filename string) error {
	rdb, err := client()
	if err != nil {
		return err
	}
	ref, err := readActiveRAGIndex(ctx, rdb, username, filename)
	if err != nil {
		return err
	}
	if ref != nil {
		if err := rdb.Do(ctx, "FT.DROPINDEX", ref.IndexName, "DD").Err(); err != nil && !isUnknownIndexError(err) {
			return fmt.Errorf("delete active RAG index: %w", err)
		}
	}
	if err := rdb.Do(ctx, "FT.DROPINDEX", GenerateUserIndexName(username, filename), "DD").Err(); err != nil && !isUnknownIndexError(err) {
		return fmt.Errorf("delete legacy RAG index: %w", err)
	}
	if err := rdb.Del(ctx, activeRAGIndexKey(username, filename)).Err(); err != nil {
		return fmt.Errorf("delete active RAG index pointer: %w", err)
	}
	return nil
}

// DeleteRedisIndex 删除 Redis 索引，支持按文件名区分
func DeleteRedisIndex(ctx context.Context, username, filename string) error {
	rdb, err := client()
	if err != nil {
		return err
	}
	if strings.TrimSpace(username) == "" || strings.TrimSpace(filename) == "" {
		return fmt.Errorf("username and filename are required")
	}
	indexName := GenerateUserIndexName(username, filename)

	// DD also removes the hashes that contain the indexed chunks. Without it,
	// replacing a file leaves orphaned vectors in Redis indefinitely.
	if err := rdb.Do(ctx, "FT.DROPINDEX", indexName, "DD").Err(); err != nil {
		if isUnknownIndexError(err) {
			return nil
		}
		return fmt.Errorf("删除索引失败: %w", err)
	}

	fmt.Println("索引删除成功！")
	return nil
}

func isUnknownIndexError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unknown index")
}
