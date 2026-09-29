package model

import "time"

// ProviderConfig stores a user's model connection. Secrets are encrypted before
// they reach this model and must never be serialized to a client.
type ProviderConfig struct {
	ID                 uint64     `gorm:"primaryKey" json:"id"`
	UserID             int64      `gorm:"index;not null;uniqueIndex:idx_provider_user_kind_name" json:"user_id"`
	Name               string     `gorm:"type:varchar(100);not null;uniqueIndex:idx_provider_user_kind_name" json:"name"`
	Kind               string     `gorm:"type:varchar(20);not null;uniqueIndex:idx_provider_user_kind_name" json:"kind"` // chat or embedding
	Protocol           string     `gorm:"type:varchar(40);not null;default:openai-compatible" json:"protocol"`
	BaseURL            string     `gorm:"type:varchar(500);not null" json:"base_url"`
	Model              string     `gorm:"type:varchar(200);not null" json:"model"`
	EncryptedAPIKey    string     `gorm:"type:text;not null" json:"-"`
	IsDefault          bool       `gorm:"not null;default:false;index" json:"is_default"`
	EmbeddingDimension int        `gorm:"default:0" json:"embedding_dimension"`
	Status             string     `gorm:"type:varchar(30);not null;default:unverified" json:"status"`
	LastTestedAt       *time.Time `json:"last_tested_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
