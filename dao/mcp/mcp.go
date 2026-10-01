package mcp

import (
	"errors"

	"github.com/dinghen/CogniGo/common/mysql"
	"github.com/dinghen/CogniGo/model"
	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

func ListServers(userID int64) ([]model.MCPServer, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	var rows []model.MCPServer
	err := mysql.DB.Where("user_id = ?", userID).Order("created_at asc").Find(&rows).Error
	return rows, err
}

func GetServer(userID int64, id uint64) (*model.MCPServer, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	var row model.MCPServer
	err := mysql.DB.Where("user_id = ? AND id = ?", userID, id).First(&row).Error
	return &row, err
}

func CreateServer(row *model.MCPServer) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	return mysql.DB.Create(row).Error
}

func UpdateServer(row *model.MCPServer) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	return mysql.DB.Model(&model.MCPServer{}).Where("user_id = ? AND id = ?", row.UserID, row.ID).Updates(row).Error
}

func DeleteServer(userID int64, id uint64) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	return mysql.DB.Transaction(func(tx *gorm.DB) error {
		var owned model.MCPServer
		if err := tx.Where("user_id = ? AND id = ?", userID, id).First(&owned).Error; err != nil {
			return err
		}
		if err := tx.Where("server_id = ?", id).Delete(&model.MCPTool{}).Error; err != nil {
			return err
		}
		result := tx.Where("user_id = ? AND id = ?", userID, id).Delete(&model.MCPServer{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func ReplaceTools(serverID uint64, tools []model.MCPTool) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	return mysql.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("server_id = ?", serverID).Delete(&model.MCPTool{}).Error; err != nil {
			return err
		}
		for i := range tools {
			if err := tx.Create(&tools[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ListTools(serverID uint64) ([]model.MCPTool, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	var rows []model.MCPTool
	err := mysql.DB.Where("server_id = ?", serverID).Order("name asc").Find(&rows).Error
	return rows, err
}

func GetTool(serverID, id uint64) (*model.MCPTool, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	var row model.MCPTool
	err := mysql.DB.Where("server_id = ? AND id = ?", serverID, id).First(&row).Error
	return &row, err
}
