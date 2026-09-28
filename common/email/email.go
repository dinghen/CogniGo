package email

import (
	"fmt"
	"github.com/dinghen/CogniGo/config"
	"log"

	"gopkg.in/gomail.v2"
)

const (
	CodeMsg     = "CogniGo验证码如下(验证码仅限于2分钟有效): "
	UserNameMsg = "CogniGo的账号如下，请保留好，后续可以用账号/邮箱登录 "
)

func SendCaptcha(email, code, msg string) error {
	settings := config.GetConfig().EmailConfig
	m := gomail.NewMessage()

	// 发件人
	from := settings.Email
	if from == "" {
		from = "no-reply@cognigo.local"
	}
	m.SetHeader("From", from)
	// 收件人
	m.SetHeader("To", email)
	// 主题
	m.SetHeader("Subject", "来自CogniGo的信息")
	// 正文内容（纯文本形式，也可以用 text/html）
	m.SetBody("text/plain", msg+" "+code)

	// 配置 SMTP 服务器和授权码,587：是 SMTP 的明文/STARTTLS 端口号
	smtpHost, smtpPort := settings.SMTPHost, settings.SMTPPort
	if smtpHost == "" {
		smtpHost = "smtp.qq.com"
	}
	if smtpPort <= 0 {
		smtpPort = 587
	}
	d := gomail.NewDialer(smtpHost, smtpPort, settings.Email, settings.Authcode)

	// 发送邮件
	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("send email through SMTP: %w", err)
	}
	log.Printf("verification email sent")
	return nil
}
