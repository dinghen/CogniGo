package provider

import (
	"errors"

	"github.com/dinghen/CogniGo/common/mysql"
	"github.com/dinghen/CogniGo/model"
	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

func List(userID int64) ([]model.ProviderConfig, error) {
	var records []model.ProviderConfig
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	err := mysql.DB.Where("user_id = ?", userID).Order("created_at asc").Find(&records).Error
	return records, err
}

func Get(userID int64, id uint64) (*model.ProviderConfig, error) {
	var record model.ProviderConfig
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	err := mysql.DB.Where("user_id = ? AND id = ?", userID, id).First(&record).Error
	return &record, err
}

func GetDefault(userID int64, kind string) (*model.ProviderConfig, error) {
	var record model.ProviderConfig
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	err := mysql.DB.Where("user_id = ? AND kind = ?", userID, kind).
		Order("is_default desc, updated_at desc").First(&record).Error
	return &record, err
}

func Create(record *model.ProviderConfig) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	return mysql.DB.Transaction(func(tx *gorm.DB) error {
		if record.IsDefault {
			if err := tx.Model(&model.ProviderConfig{}).Where("user_id = ? AND kind = ?", record.UserID, record.Kind).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(record).Error
	})
}

func Update(record *model.ProviderConfig) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	return mysql.DB.Transaction(func(tx *gorm.DB) error {
		if record.IsDefault {
			if err := tx.Model(&model.ProviderConfig{}).Where("user_id = ? AND kind = ? AND id <> ?", record.UserID, record.Kind, record.ID).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.ProviderConfig{}).Where("user_id = ? AND id = ?", record.UserID, record.ID).Updates(record).Error
	})
}

func Delete(userID int64, id uint64) error {
	if mysql.DB == nil {
		return errors.New("database is not initialized")
	}
	result := mysql.DB.Where("user_id = ? AND id = ?", userID, id).Delete(&model.ProviderConfig{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
