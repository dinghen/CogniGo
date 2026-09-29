package provider

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"
	"github.com/dinghen/CogniGo/common/mysql"
	"github.com/dinghen/CogniGo/config"
	providerDAO "github.com/dinghen/CogniGo/dao/provider"
	"github.com/dinghen/CogniGo/dao/user"
	"github.com/dinghen/CogniGo/model"
	"gorm.io/gorm"
)

var (
	ErrInvalidKind       = errors.New("provider kind must be chat or embedding")
	ErrInvalidInput      = errors.New("provider input is invalid")
	ErrEncryptionKey     = errors.New("COGNIGO_PROVIDER_ENCRYPTION_KEY must be a base64-encoded or 32-byte key")
	ErrSecretUnavailable = errors.New("provider API key is unavailable")
)

type ResolvedConfig struct {
	Name      string
	Kind      string
	Protocol  string
	BaseURL   string
	Model     string
	APIKey    string
	Dimension int
	FromUser  bool
}

type ProviderDTO struct {
	ID                 uint64     `json:"id"`
	Name               string     `json:"name"`
	Kind               string     `json:"kind"`
	Protocol           string     `json:"protocol"`
	BaseURL            string     `json:"base_url"`
	Model              string     `json:"model"`
	APIKeyMasked       string     `json:"api_key_masked"`
	HasAPIKey          bool       `json:"has_api_key"`
	IsDefault          bool       `json:"is_default"`
	EmbeddingDimension int        `json:"embedding_dimension,omitempty"`
	Status             string     `json:"status"`
	LastTestedAt       *time.Time `json:"last_tested_at,omitempty"`
}

type Input struct {
	Name      string `json:"name" binding:"required"`
	Kind      string `json:"kind" binding:"required"`
	Protocol  string `json:"protocol"`
	BaseURL   string `json:"base_url" binding:"required,url"`
	Model     string `json:"model" binding:"required"`
	APIKey    string `json:"api_key"`
	IsDefault bool   `json:"is_default"`
}

func masterKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("COGNIGO_PROVIDER_ENCRYPTION_KEY"))
	if raw == "" {
		return nil, ErrEncryptionKey
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len(raw) == 32 {
		return []byte(raw), nil
	}
	return nil, ErrEncryptionKey
}

func EncryptSecret(secret string) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", nil
	}
	key, err := masterKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create secret cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create secret AEAD: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate secret nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(secret), nil)
	return base64.RawStdEncoding.EncodeToString(ciphertext), nil
}

func DecryptSecret(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	key, err := masterKey()
	if err != nil {
		return "", err
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrSecretUnavailable
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ErrSecretUnavailable
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", ErrSecretUnavailable
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", ErrSecretUnavailable
	}
	return string(plain), nil
}

func MaskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	if len([]rune(secret)) <= 8 {
		return "********"
	}
	runes := []rune(secret)
	return string(runes[:4]) + "..." + string(runes[len(runes)-4:])
}

func normalizeKind(kind string) string { return strings.ToLower(strings.TrimSpace(kind)) }

func validateInput(input Input) error {
	input.Kind = normalizeKind(input.Kind)
	if input.Kind != "chat" && input.Kind != "embedding" {
		return ErrInvalidKind
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Model) == "" || strings.TrimSpace(input.BaseURL) == "" {
		return ErrInvalidInput
	}
	return nil
}

func userID(username string) (int64, error) {
	if mysql.DB == nil {
		return 0, errors.New("database is not initialized")
	}
	record, err := user.FindByUsername(username)
	if err != nil {
		return 0, err
	}
	if record == nil {
		return 0, providerDAO.ErrNotFound
	}
	return record.ID, nil
}

func toDTO(record *model.ProviderConfig) ProviderDTO {
	return ProviderDTO{ID: record.ID, Name: record.Name, Kind: record.Kind, Protocol: record.Protocol, BaseURL: record.BaseURL, Model: record.Model, APIKeyMasked: secretMaskFromCipher(record.EncryptedAPIKey), HasAPIKey: record.EncryptedAPIKey != "", IsDefault: record.IsDefault, EmbeddingDimension: record.EmbeddingDimension, Status: record.Status, LastTestedAt: record.LastTestedAt}
}

func secretMaskFromCipher(encoded string) string {
	if encoded == "" {
		return ""
	}
	return "********"
}

func List(username string) ([]ProviderDTO, error) {
	id, err := userID(username)
	if err != nil {
		return nil, err
	}
	records, err := providerDAO.List(id)
	if err != nil {
		return nil, err
	}
	result := make([]ProviderDTO, 0, len(records))
	for i := range records {
		result = append(result, toDTO(&records[i]))
	}
	return result, nil
}

func Create(username string, input Input) (ProviderDTO, error) {
	if err := validateInput(input); err != nil {
		return ProviderDTO{}, err
	}
	id, err := userID(username)
	if err != nil {
		return ProviderDTO{}, err
	}
	secret, err := EncryptSecret(input.APIKey)
	if err != nil {
		return ProviderDTO{}, err
	}
	protocol := input.Protocol
	if protocol == "" {
		protocol = "openai-compatible"
	}
	record := &model.ProviderConfig{UserID: id, Name: strings.TrimSpace(input.Name), Kind: normalizeKind(input.Kind), Protocol: protocol, BaseURL: strings.TrimRight(strings.TrimSpace(input.BaseURL), "/"), Model: strings.TrimSpace(input.Model), EncryptedAPIKey: secret, IsDefault: input.IsDefault, Status: "unverified"}
	if err := providerDAO.Create(record); err != nil {
		return ProviderDTO{}, err
	}
	return toDTO(record), nil
}

func Update(username string, id uint64, input Input) (ProviderDTO, error) {
	if err := validateInput(input); err != nil {
		return ProviderDTO{}, err
	}
	userID, err := userID(username)
	if err != nil {
		return ProviderDTO{}, err
	}
	record, err := providerDAO.Get(userID, id)
	if err != nil {
		return ProviderDTO{}, providerDAO.ErrNotFound
	}
	secret := record.EncryptedAPIKey
	if strings.TrimSpace(input.APIKey) != "" {
		secret, err = EncryptSecret(input.APIKey)
		if err != nil {
			return ProviderDTO{}, err
		}
	}
	protocol := input.Protocol
	if protocol == "" {
		protocol = "openai-compatible"
	}
	oldKind, oldModel, oldURL := record.Kind, record.Model, record.BaseURL
	record.Name, record.Kind, record.Protocol, record.BaseURL, record.Model, record.EncryptedAPIKey, record.IsDefault = strings.TrimSpace(input.Name), normalizeKind(input.Kind), protocol, strings.TrimRight(strings.TrimSpace(input.BaseURL), "/"), strings.TrimSpace(input.Model), secret, input.IsDefault
	if oldKind == "embedding" && (oldModel != record.Model || oldURL != record.BaseURL) {
		record.Status = "rebuild_required"
		record.EmbeddingDimension = 0
	}
	if err := providerDAO.Update(record); err != nil {
		return ProviderDTO{}, err
	}
	return toDTO(record), nil
}

func Delete(username string, id uint64) error {
	uid, err := userID(username)
	if err != nil {
		return err
	}
	return providerDAO.Delete(uid, id)
}

func Get(username string, id uint64) (ProviderDTO, error) {
	uid, err := userID(username)
	if err != nil {
		return ProviderDTO{}, err
	}
	record, err := providerDAO.Get(uid, id)
	if err != nil {
		return ProviderDTO{}, providerDAO.ErrNotFound
	}
	return toDTO(record), nil
}

func ResolveByID(username string, id uint64) (ResolvedConfig, error) {
	uid, err := userID(username)
	if err != nil {
		return ResolvedConfig{}, err
	}
	record, err := providerDAO.Get(uid, id)
	if err != nil {
		return ResolvedConfig{}, providerDAO.ErrNotFound
	}
	secret, err := DecryptSecret(record.EncryptedAPIKey)
	if err != nil && record.EncryptedAPIKey != "" {
		return ResolvedConfig{}, err
	}
	return ResolvedConfig{Name: record.Name, Kind: record.Kind, Protocol: record.Protocol, BaseURL: record.BaseURL, Model: record.Model, APIKey: secret, Dimension: record.EmbeddingDimension, FromUser: true}, nil
}

func TestChat(ctx context.Context, cfg ResolvedConfig) error {
	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey})
	if err != nil {
		return fmt.Errorf("create chat model: %w", err)
	}
	if _, err := llm.Generate(ctx, []*schema.Message{{Role: schema.User, Content: "Reply with OK."}}); err != nil {
		return fmt.Errorf("chat provider test: %w", err)
	}
	return nil
}

func Resolve(username, kind string) (ResolvedConfig, error) {
	uid, err := userID(username)
	if err == nil {
		record, getErr := providerDAO.GetDefault(uid, normalizeKind(kind))
		if getErr == nil {
			secret, decryptErr := DecryptSecret(record.EncryptedAPIKey)
			if decryptErr != nil && record.EncryptedAPIKey != "" {
				return ResolvedConfig{}, decryptErr
			}
			return ResolvedConfig{Name: record.Name, Kind: record.Kind, Protocol: record.Protocol, BaseURL: record.BaseURL, Model: record.Model, APIKey: secret, Dimension: record.EmbeddingDimension, FromUser: true}, nil
		}
		if !errors.Is(getErr, providerDAO.ErrNotFound) {
			return ResolvedConfig{}, getErr
		}
	} else if !errors.Is(err, providerDAO.ErrNotFound) && !errors.Is(err, gorm.ErrRecordNotFound) {
		return ResolvedConfig{}, err
	}
	cfg := config.GetConfig()
	if kind == "embedding" {
		return ResolvedConfig{Name: "environment", Kind: "embedding", Protocol: "openai-compatible", BaseURL: cfg.RagModelConfig.RagBaseUrl, Model: cfg.RagModelConfig.RagEmbeddingModel, APIKey: os.Getenv("OPENAI_API_KEY")}, nil
	}
	return ResolvedConfig{Name: "environment", Kind: "chat", Protocol: "openai-compatible", BaseURL: os.Getenv("OPENAI_BASE_URL"), Model: os.Getenv("OPENAI_MODEL_NAME"), APIKey: os.Getenv("OPENAI_API_KEY")}, nil
}

func MarkEmbeddingProbe(username string, id uint64, dimension int) (ProviderDTO, error) {
	uid, err := userID(username)
	if err != nil {
		return ProviderDTO{}, err
	}
	record, err := providerDAO.Get(uid, id)
	if err != nil {
		return ProviderDTO{}, providerDAO.ErrNotFound
	}
	if record.Kind != "embedding" {
		return ProviderDTO{}, ErrInvalidKind
	}
	// A successful probe proves the provider responds, but does not prove the
	// active Redis index has been rebuilt for this identity. The RAG task owns
	// the explicit rebuild and activation lifecycle.
	record.EmbeddingDimension, record.Status = dimension, "probe_success"
	now := time.Now()
	record.LastTestedAt = &now
	if err := providerDAO.Update(record); err != nil {
		return ProviderDTO{}, err
	}
	return toDTO(record), nil
}

// MarkEmbeddingRebuilt records that a compatible Redis generation is active.
// The caller must invoke this only after the rebuild has completed successfully.
func MarkEmbeddingRebuilt(username string, id uint64, dimension int) (ProviderDTO, error) {
	if dimension <= 0 {
		return ProviderDTO{}, errors.New("embedding dimension must be positive")
	}
	uid, err := userID(username)
	if err != nil {
		return ProviderDTO{}, err
	}
	record, err := providerDAO.Get(uid, id)
	if err != nil {
		return ProviderDTO{}, providerDAO.ErrNotFound
	}
	if record.Kind != "embedding" {
		return ProviderDTO{}, ErrInvalidKind
	}
	record.EmbeddingDimension, record.Status = dimension, "ready"
	now := time.Now()
	record.LastTestedAt = &now
	if err := providerDAO.Update(record); err != nil {
		return ProviderDTO{}, err
	}
	return toDTO(record), nil
}
