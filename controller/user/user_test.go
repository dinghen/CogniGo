package user

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthHandlersRejectMissingRequiredFields(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	router := gin.New()
	router.POST("/captcha", HandleCaptcha)
	router.POST("/register", Register)
	router.POST("/login", Login)

	for _, path := range []string{"/captcha", "/register", "/login"} {
		path := path
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status_code":2001`) {
				t.Fatalf("response = (%d, %s), want invalid-parameters response", response.Code, response.Body.String())
			}
		})
	}
}
