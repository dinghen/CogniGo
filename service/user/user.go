package user

import (
	"log"

	"github.com/dinghen/CogniGo/common/code"
	myemail "github.com/dinghen/CogniGo/common/email"
	myredis "github.com/dinghen/CogniGo/common/redis"
	"github.com/dinghen/CogniGo/dao/user"
	"github.com/dinghen/CogniGo/utils"
	"github.com/dinghen/CogniGo/utils/myjwt"
)

func Login(identifier, password string) (string, code.Code) {
	userInformation, err := user.FindByIdentifier(identifier)
	if err != nil {
		log.Printf("login user lookup failed: %T", err)
		return "", code.CodeServerBusy
	}
	if userInformation == nil {
		return "", code.CodeUserNotExist
	}
	//2:判断用户是否密码账号正确
	if userInformation.Password != utils.MD5(password) {
		return "", code.CodeInvalidPassword
	}
	//3:返回一个Token
	token, err := myjwt.GenerateToken(userInformation.ID, userInformation.Username)

	if err != nil {
		return "", code.CodeServerBusy
	}
	return token, code.CodeSuccess
}

func Register(email, password, captcha string) (string, code.Code) {
	existing, err := user.FindByIdentifier(email)
	if err != nil {
		log.Printf("registration user lookup failed: %T", err)
		return "", code.CodeServerBusy
	}
	if existing != nil {
		return "", code.CodeUserExist
	}

	//2:从redis中验证验证码是否有效
	valid, err := myredis.CheckCaptchaForEmail(email, captcha)
	if err != nil {
		log.Printf("registration captcha lookup failed: %T", err)
		return "", code.CodeCaptchaServiceUnavailable
	}
	if !valid {
		return "", code.CodeInvalidCaptcha
	}

	//3：生成11位的账号
	username := utils.GetRandomNumbers(11)

	//4：注册到数据库中
	userInformation, err := user.Register(username, email, password)
	if err != nil {
		if restoreErr := myredis.RestoreCaptchaForEmail(email, captcha); restoreErr != nil {
			log.Printf("registration captcha restore failed: %T", restoreErr)
		}
		log.Printf("registration database insert failed: %T", err)
		return "", code.CodeServerBusy
	}

	//5：将账号一并发送到对应邮箱上去，后续需要账号登录
	if err := myemail.SendCaptcha(email, username, user.UserNameMsg); err != nil {
		// The account is already durable and can also be used with the
		// registration email, so notification delivery must not report a false
		// registration failure.
		log.Printf("registration username notification failed for user id %d", userInformation.ID)
	}

	// 6:生成Token
	token, err := myjwt.GenerateToken(userInformation.ID, userInformation.Username)

	if err != nil {
		return "", code.CodeServerBusy
	}

	return token, code.CodeSuccess
}

// 往指定邮箱发送验证码
// 分为以下任务：
// 1：先存放redis
// 2：再进行远程发送
func SendCaptcha(email_ string) code.Code {
	send_code := utils.GetRandomNumbers(6)
	//1:先存放到redis
	if err := myredis.SetCaptchaForEmail(email_, send_code); err != nil {
		log.Printf("verification captcha storage failed: %T", err)
		return code.CodeCaptchaServiceUnavailable
	}

	//2:再进行远程发送
	if err := myemail.SendCaptcha(email_, send_code, myemail.CodeMsg); err != nil {
		if deleteErr := myredis.DeleteCaptchaForEmail(email_, send_code); deleteErr != nil {
			log.Printf("verification captcha cleanup failed: %T", deleteErr)
		}
		log.Printf("verification email delivery failed: %T", err)
		return code.CodeCaptchaDeliveryFailed
	}

	return code.CodeSuccess
}
