package redis

import (
	"context"
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

	fmt.Println("正在创建 Redis 索引...")

	prefix := GenerateUserIndexNamePrefix(username, filename)

	// 创建索引
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
		return fmt.Errorf("创建索引失败: %w", err)
	}

	fmt.Println("索引创建成功！")
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
