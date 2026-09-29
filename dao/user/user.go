package user

import (
	"context"
	"errors"
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

// 这边只能通过账号进行登录
func IsExistUser(username string) (bool, *model.User) {

	user, err := mysql.GetUserByUsername(username)

	if err == gorm.ErrRecordNotFound || user == nil {
		return false, nil
	}

	return true, user
}

// FindByUsername returns the authenticated user's record for ownership checks.
func FindByUsername(username string) (*model.User, error) {
	if mysql.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	return mysql.GetUserByUsername(username)
}

func Register(username, email, password string) (*model.User, bool) {
	if user, err := mysql.InsertUser(&model.User{
		Email:    email,
		Name:     username,
		Username: username,
		Password: utils.MD5(password),
	}); err != nil {
		return nil, false
	} else {
		return user, true
	}
}
