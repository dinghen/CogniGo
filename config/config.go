package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/BurntSushi/toml"
)

type MainConfig struct {
	Port    int    `toml:"port"`
	AppName string `toml:"appName"`
	Host    string `toml:"host"`
}

type EmailConfig struct {
	Authcode string `toml:"authcode"`
	Email    string `toml:"email" `
	SMTPHost string `toml:"smtpHost"`
	SMTPPort int    `toml:"smtpPort"`
}

type RedisConfig struct {
	RedisPort     int    `toml:"port"`
	RedisDb       int    `toml:"db"`
	RedisHost     string `toml:"host"`
	RedisPassword string `toml:"password"`
}

type MysqlConfig struct {
	MysqlPort         int    `toml:"port"`
	MysqlHost         string `toml:"host"`
	MysqlUser         string `toml:"user"`
	MysqlPassword     string `toml:"password"`
	MysqlDatabaseName string `toml:"databaseName"`
	MysqlCharset      string `toml:"charset"`
}

type JwtConfig struct {
	ExpireDuration int    `toml:"expire_duration"`
	Issuer         string `toml:"issuer"`
	Subject        string `toml:"subject"`
	Key            string `toml:"key"`
}

type Rabbitmq struct {
	RabbitmqPort     int    `toml:"port"`
	RabbitmqHost     string `toml:"host"`
	RabbitmqUsername string `toml:"username"`
	RabbitmqPassword string `toml:"password"`
	RabbitmqVhost    string `toml:"vhost"`
}

type RagModelConfig struct {
	RagEmbeddingModel       string  `toml:"embeddingModel"`
	RagChatModelName        string  `toml:"chatModelName"`
	RagDocDir               string  `toml:"docDir"`
	RagBaseUrl              string  `toml:"baseUrl"`
	RagChunkSize            int     `toml:"chunkSize"`
	RagChunkOverlap         int     `toml:"chunkOverlap"`
	RagTopK                 int     `toml:"topK"`
	RagDistanceThreshold    float64 `toml:"distanceThreshold"`
	RagUseDistanceThreshold bool    `toml:"useDistanceThreshold"`
}

type VoiceServiceConfig struct {
	VoiceServiceApiKey    string `toml:"voiceServiceApiKey"`
	VoiceServiceSecretKey string `toml:"voiceServiceSecretKey"`
}

type RuntimeConfig struct {
	UploadDir       string `toml:"uploadDir"`
	ONNXLibraryPath string `toml:"onnxLibraryPath"`
	ModelPath       string `toml:"modelPath"`
	LabelsPath      string `toml:"labelsPath"`
	MCPURL          string `toml:"mcpURL"`
	MCPMaxSteps     int    `toml:"mcpMaxSteps"`
}

type Config struct {
	EmailConfig           `toml:"emailConfig"`
	RedisConfig           `toml:"redisConfig"`
	MysqlConfig           `toml:"mysqlConfig"`
	JwtConfig             `toml:"jwtConfig"`
	MainConfig            `toml:"mainConfig"`
	Rabbitmq              `toml:"rabbitmqConfig"`
	RagModelConfig        `toml:"ragModelConfig"`
	VoiceServiceConfig    `toml:"voiceServiceConfig"`
	RuntimeConfig         `toml:"runtimeConfig"`
	ProviderEncryptionKey string `toml:"-"`
}

type RedisKeyConfig struct {
	CaptchaPrefix   string
	IndexName       string
	IndexNamePrefix string
}

var DefaultRedisKeyConfig = RedisKeyConfig{
	CaptchaPrefix:   "captcha:%s",
	IndexName:       "rag_docs:%s:idx",
	IndexNamePrefix: "rag_docs:%s:",
}

var config *Config

// InitConfig 初始化项目配置
func InitConfig() error {
	if config == nil {
		config = new(Config)
	}
	// 设置配置文件路径（相对于 main.go 所在的目录）
	if _, err := toml.DecodeFile("config/config.toml", config); err != nil {
		return err
	}
	return applyEnvironment(config)
}

func applyEnvironment(c *Config) error {
	setString := func(dst *string, key string) {
		if value, ok := os.LookupEnv(key); ok {
			*dst = value
		}
	}
	setInt := func(dst *int, key string) error {
		value, ok := os.LookupEnv(key)
		if !ok {
			return nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s must be an integer: %w", key, err)
		}
		*dst = parsed
		return nil
	}

	setString(&c.MainConfig.AppName, "COGNIGO_APP_NAME")
	setString(&c.MainConfig.Host, "COGNIGO_HOST")
	if err := setInt(&c.MainConfig.Port, "COGNIGO_PORT"); err != nil {
		return err
	}
	setString(&c.EmailConfig.Email, "COGNIGO_EMAIL")
	setString(&c.EmailConfig.Authcode, "COGNIGO_EMAIL_AUTHCODE")
	setString(&c.EmailConfig.SMTPHost, "COGNIGO_SMTP_HOST")
	if err := setInt(&c.EmailConfig.SMTPPort, "COGNIGO_SMTP_PORT"); err != nil {
		return err
	}
	if c.EmailConfig.SMTPHost == "" {
		c.EmailConfig.SMTPHost = "localhost"
	}
	if c.EmailConfig.SMTPPort <= 0 {
		c.EmailConfig.SMTPPort = 1025
	}

	setString(&c.RedisConfig.RedisHost, "COGNIGO_REDIS_HOST")
	if err := setInt(&c.RedisConfig.RedisPort, "COGNIGO_REDIS_PORT"); err != nil {
		return err
	}
	if err := setInt(&c.RedisConfig.RedisDb, "COGNIGO_REDIS_DB"); err != nil {
		return err
	}
	setString(&c.RedisConfig.RedisPassword, "COGNIGO_REDIS_PASSWORD")

	setString(&c.MysqlConfig.MysqlHost, "COGNIGO_MYSQL_HOST")
	if err := setInt(&c.MysqlConfig.MysqlPort, "COGNIGO_MYSQL_PORT"); err != nil {
		return err
	}
	setString(&c.MysqlConfig.MysqlUser, "COGNIGO_MYSQL_USER")
	setString(&c.MysqlConfig.MysqlPassword, "COGNIGO_MYSQL_PASSWORD")
	setString(&c.MysqlConfig.MysqlDatabaseName, "COGNIGO_MYSQL_DATABASE")
	setString(&c.MysqlConfig.MysqlCharset, "COGNIGO_MYSQL_CHARSET")

	setString(&c.JwtConfig.Issuer, "COGNIGO_JWT_ISSUER")
	setString(&c.JwtConfig.Subject, "COGNIGO_JWT_SUBJECT")
	setString(&c.JwtConfig.Key, "COGNIGO_JWT_KEY")
	if err := setInt(&c.JwtConfig.ExpireDuration, "COGNIGO_JWT_EXPIRE_HOURS"); err != nil {
		return err
	}

	setString(&c.Rabbitmq.RabbitmqHost, "COGNIGO_RABBITMQ_HOST")
	if err := setInt(&c.Rabbitmq.RabbitmqPort, "COGNIGO_RABBITMQ_PORT"); err != nil {
		return err
	}
	setString(&c.Rabbitmq.RabbitmqUsername, "COGNIGO_RABBITMQ_USER")
	setString(&c.Rabbitmq.RabbitmqPassword, "COGNIGO_RABBITMQ_PASSWORD")
	setString(&c.Rabbitmq.RabbitmqVhost, "COGNIGO_RABBITMQ_VHOST")

	setString(&c.RagModelConfig.RagEmbeddingModel, "COGNIGO_EMBEDDING_MODEL")
	setString(&c.RagModelConfig.RagChatModelName, "COGNIGO_RAG_CHAT_MODEL")
	setString(&c.RagModelConfig.RagDocDir, "COGNIGO_RAG_DOC_DIR")
	setString(&c.RagModelConfig.RagBaseUrl, "COGNIGO_RAG_BASE_URL")
	if err := setInt(&c.RagModelConfig.RagChunkSize, "COGNIGO_RAG_CHUNK_SIZE"); err != nil {
		return err
	}
	if err := setInt(&c.RagModelConfig.RagChunkOverlap, "COGNIGO_RAG_CHUNK_OVERLAP"); err != nil {
		return err
	}
	if err := setInt(&c.RagModelConfig.RagTopK, "COGNIGO_RAG_TOP_K"); err != nil {
		return err
	}
	if value, ok := os.LookupEnv("COGNIGO_RAG_DISTANCE_THRESHOLD"); ok {
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("COGNIGO_RAG_DISTANCE_THRESHOLD must be a number: %w", err)
		}
		c.RagModelConfig.RagDistanceThreshold = parsed
	}
	if value, ok := os.LookupEnv("COGNIGO_RAG_USE_DISTANCE_THRESHOLD"); ok {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("COGNIGO_RAG_USE_DISTANCE_THRESHOLD must be a boolean: %w", err)
		}
		c.RagModelConfig.RagUseDistanceThreshold = parsed
	}

	setString(&c.VoiceServiceConfig.VoiceServiceApiKey, "COGNIGO_BAIDU_API_KEY")
	setString(&c.VoiceServiceConfig.VoiceServiceSecretKey, "COGNIGO_BAIDU_SECRET_KEY")
	setString(&c.RuntimeConfig.UploadDir, "COGNIGO_UPLOAD_DIR")
	setString(&c.RuntimeConfig.ONNXLibraryPath, "COGNIGO_ONNX_LIBRARY_PATH")
	setString(&c.RuntimeConfig.ModelPath, "COGNIGO_ONNX_MODEL_PATH")
	setString(&c.RuntimeConfig.LabelsPath, "COGNIGO_LABELS_PATH")
	setString(&c.RuntimeConfig.MCPURL, "COGNIGO_MCP_URL")
	setString(&c.ProviderEncryptionKey, "COGNIGO_PROVIDER_ENCRYPTION_KEY")
	if err := setInt(&c.RuntimeConfig.MCPMaxSteps, "COGNIGO_MCP_MAX_STEPS"); err != nil {
		return err
	}
	return nil
}

func GetConfig() *Config {
	if config == nil {
		config = new(Config)
		if err := InitConfig(); err != nil {
			log.Printf("load config failed: %v", err)
		}
	}
	return config
}
