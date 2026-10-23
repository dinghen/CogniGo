package user

import (
	"testing"

	"github.com/dinghen/CogniGo/common/code"
	"github.com/dinghen/CogniGo/common/mysql"
	myredis "github.com/dinghen/CogniGo/common/redis"
)

func TestLoginReturnsServiceErrorWhenDatabaseIsUnavailable(t *testing.T) {
	previous := mysql.DB
	mysql.DB = nil
	t.Cleanup(func() { mysql.DB = previous })

	if _, got := Login("person@example.com", "password"); got != code.CodeServerBusy {
		t.Fatalf("Login code = %v, want %v", got, code.CodeServerBusy)
	}
}

func TestSendCaptchaDistinguishesRedisInfrastructureFailure(t *testing.T) {
	previous := myredis.Rdb
	myredis.Rdb = nil
	t.Cleanup(func() { myredis.Rdb = previous })

	if got := SendCaptcha("person@example.com"); got != code.CodeCaptchaServiceUnavailable {
		t.Fatalf("SendCaptcha code = %v, want %v", got, code.CodeCaptchaServiceUnavailable)
	}
}
