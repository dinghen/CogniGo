package knowledge

import (
	"errors"
	"github.com/dinghen/CogniGo/common/mysql"
	userDAO "github.com/dinghen/CogniGo/dao/user"
	"github.com/dinghen/CogniGo/model"
	"gorm.io/gorm"
)

func Upsert(record *model.KnowledgeFile) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	if record.UserID == 0 {
		user, err := userDAO.FindByUsername(record.Username)
		if err != nil {
			return err
		}
		record.UserID = user.ID
	}
	var existing model.KnowledgeFile
	err := mysql.DB.Where("username = ? AND filename = ?", record.Username, record.Filename).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return mysql.DB.Create(record).Error
	}
	if err != nil {
		return err
	}
	record.ID = existing.ID
	return mysql.DB.Model(&existing).Updates(record).Error
}

func UpdateStatus(username, filename, status, summary, generation, fingerprint string) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	result := mysql.DB.Model(&model.KnowledgeFile{}).Where("username = ? AND filename = ?", username, filename).Updates(map[string]any{"status": status, "error_summary": summary, "generation": generation, "embedding_fingerprint": fingerprint})
	return result.Error
}

func Delete(username, filename string) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	return mysql.DB.Where("username = ? AND filename = ?", username, filename).Delete(&model.KnowledgeFile{}).Error
}

func List(username string) ([]model.KnowledgeFile, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	var records []model.KnowledgeFile
	err := mysql.DB.Where("username = ?", username).Order("filename asc").Find(&records).Error
	return records, err
}

func MarkStale(username string) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	return mysql.DB.Model(&model.KnowledgeFile{}).Where("username = ? AND status = ?", username, model.KnowledgeReady).Update("status", model.KnowledgeStale).Error
}
