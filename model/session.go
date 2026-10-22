package model

import (
	"time"

	"gorm.io/gorm"
)

type Session struct {
	ID                  string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	UserName            string         `gorm:"index;not null" json:"username"`
	Title               string         `gorm:"type:varchar(100)" json:"title"`
	MCPServerIDsJSON    string         `gorm:"type:text" json:"-"`
	MCPToolIDsJSON      string         `gorm:"type:text" json:"-"`
	ChatProviderID      *uint64        `gorm:"index" json:"-"`
	EmbeddingProviderID *uint64        `gorm:"index" json:"-"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

type SessionInfo struct {
	SessionID           string   `json:"sessionId"`
	Title               string   `json:"name"`
	ChatProviderID      *uint64  `json:"chatProviderId"`
	EmbeddingProviderID *uint64  `json:"embeddingProviderId"`
	MCPServerIDs        []uint64 `json:"mcpServerIds"`
	MCPToolIDs          []uint64 `json:"mcpToolIds"`
}
