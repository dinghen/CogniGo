package session

import (
	"errors"

	"github.com/dinghen/CogniGo/common/mysql"
	"github.com/dinghen/CogniGo/model"
	"gorm.io/gorm"
)

func GetSessionsByUserName(UserName int64) ([]model.Session, error) {
	var sessions []model.Session
	err := mysql.DB.Where("user_name = ?", UserName).Find(&sessions).Error
	return sessions, err
}

func GetSessionsByUsername(username string) ([]model.Session, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	var sessions []model.Session
	err := mysql.DB.Where("user_name = ?", username).Order("updated_at desc").Find(&sessions).Error
	return sessions, err
}

func CreateSession(session *model.Session) (*model.Session, error) {
	err := mysql.DB.Create(session).Error
	return session, err
}

func GetSessionByID(sessionID string) (*model.Session, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	var session model.Session
	err := mysql.DB.Where("id = ?", sessionID).First(&session).Error
	return &session, err
}

// GetSessionByIDForUser prevents a session ID from being used to resolve
// another user's provider selections or message history.
func GetSessionByIDForUser(userName, sessionID string) (*model.Session, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	var session model.Session
	err := mysql.DB.Where("id = ? AND user_name = ?", sessionID, userName).First(&session).Error
	return &session, err
}

func UpdateProviderSelections(userName, sessionID string, chatProviderID, embeddingProviderID *uint64) error {
	return UpdateCapabilitySelections(userName, sessionID, chatProviderID, embeddingProviderID, nil, nil)
}

// UpdateCapabilitySelections persists the complete per-session capability set.
// Nil MCP slices mean the caller did not request a MCP selection change; an
// empty, non-nil slice intentionally clears the existing selection.
func UpdateCapabilitySelections(userName, sessionID string, chatProviderID, embeddingProviderID *uint64, mcpServerIDsJSON, mcpToolIDsJSON *string) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	updates := map[string]interface{}{"chat_provider_id": chatProviderID, "embedding_provider_id": embeddingProviderID}
	if mcpServerIDsJSON != nil {
		updates["mcp_server_ids_json"] = *mcpServerIDsJSON
	}
	if mcpToolIDsJSON != nil {
		updates["mcp_tool_ids_json"] = *mcpToolIDsJSON
	}
	result := mysql.DB.Model(&model.Session{}).Where("id = ? AND user_name = ?", sessionID, userName).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
