package model

import "time"

// MCPServer is a user-owned MCP connection profile. Secrets are encrypted
// before persistence and are never serialized through the API model.
type MCPServer struct {
	ID               uint64     `gorm:"primaryKey" json:"id"`
	UserID           int64      `gorm:"index;not null;uniqueIndex:idx_mcp_server_user_name" json:"user_id"`
	Name             string     `gorm:"type:varchar(100);not null;uniqueIndex:idx_mcp_server_user_name" json:"name"`
	Transport        string     `gorm:"type:varchar(30);not null" json:"transport"`
	URL              string     `gorm:"type:varchar(1000)" json:"url,omitempty"`
	EncryptedHeaders string     `gorm:"type:text" json:"-"`
	Command          string     `gorm:"type:varchar(255)" json:"command,omitempty"`
	ArgsJSON         string     `gorm:"type:text" json:"-"`
	EncryptedEnv     string     `gorm:"type:text" json:"-"`
	Enabled          bool       `gorm:"not null;default:true" json:"enabled"`
	Status           string     `gorm:"type:varchar(30);not null;default:unverified" json:"status"`
	LastTestedAt     *time.Time `json:"last_tested_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// MCPTool is the last discovered catalog entry for a server.
type MCPTool struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	ServerID        uint64    `gorm:"index;not null;uniqueIndex:idx_mcp_tool_server_name" json:"server_id"`
	Name            string    `gorm:"type:varchar(200);not null;uniqueIndex:idx_mcp_tool_server_name" json:"name"`
	Description     string    `gorm:"type:text" json:"description,omitempty"`
	InputSchemaJSON string    `gorm:"type:text" json:"input_schema,omitempty"`
	Enabled         bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
