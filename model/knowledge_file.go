package model

import "time"

const (
	KnowledgePending  = "pending"
	KnowledgeIndexing = "indexing"
	KnowledgeReady    = "ready"
	KnowledgeStale    = "stale"
	KnowledgeFailed   = "failed"
)

// KnowledgeFile is user-owned metadata for one uploaded source. Raw content
// remains under the validated user directory; this row tracks its lifecycle.
type KnowledgeFile struct {
	ID                   uint64    `gorm:"primaryKey" json:"id"`
	UserID               int64     `gorm:"index;not null" json:"user_id"`
	Username             string    `gorm:"type:varchar(100);not null;index;uniqueIndex:idx_knowledge_user_file" json:"username"`
	Filename             string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_knowledge_user_file" json:"filename"`
	Status               string    `gorm:"type:varchar(20);not null;index" json:"status"`
	ErrorSummary         string    `gorm:"type:varchar(500)" json:"error_summary,omitempty"`
	Generation           string    `gorm:"type:varchar(100)" json:"generation,omitempty"`
	EmbeddingFingerprint string    `gorm:"type:varchar(128)" json:"embedding_fingerprint,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}
