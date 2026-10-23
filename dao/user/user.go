package user

import (
	"context"
	"errors"
	"fmt"
	"github.com/dinghen/CogniGo/common/mysql"
	"github.com/dinghen/CogniGo/model"
	"github.com/dinghen/CogniGo/utils"

	"gorm.io/gorm"
)

const (
	CodeMsg     = "CogniGo验证码如下(验证码仅限于2分钟有效): "
	UserNameMsg = "CogniGo的账号如下，请保留好，后续可以用账号进行登录 "
)

var ctx = context.Background()

// FindByIdentifier accepts either the generated username or registration email.
// A nil user means no matching record; database errors remain distinguishable.
func FindByIdentifier(identifier string) (*model.User, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	user, err := mysql.GetUserByIdentifier(identifier)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user by identifier: %w", err)
	}
	return user, nil
}

// FindByUsername returns the authenticated user's record for ownership checks.
func FindByUsername(username string) (*model.User, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	return mysql.GetUserByUsername(username)
}

func Register(username, email, password string) (*model.User, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	user, err := mysql.InsertUser(&model.User{
		Email:    email,
		Name:     username,
		Username: username,
		Password: utils.MD5(password),
	})
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	return user, nil
}
