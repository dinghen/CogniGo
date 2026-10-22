package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCreateReturnsActionableMapValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{name: "headers", body: `{"name":"demo","transport":"streamable-http","url":"https://example.com/mcp","headers":{"Authorization":true}}`, want: `headers["Authorization"]`},
		{name: "env", body: `{"name":"demo","transport":"stdio","command":"node","env":{"TOKEN":[]}}`, want: `env["TOKEN"]`},
		{name: "malformed JSON", body: `{"name":"demo",`, want: "MCP configuration is invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/mcp-servers", strings.NewReader(test.body))
			ctx.Request.Header.Set("Content-Type", "application/json")

			Create(ctx)

			if writer.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body: %s", writer.Code, http.StatusBadRequest, writer.Body.String())
			}
			var result struct {
				StatusMsg string `json:"status_msg"`
			}
			if err := json.Unmarshal(writer.Body.Bytes(), &result); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if !strings.Contains(result.StatusMsg, test.want) {
				t.Fatalf("status_msg = %q, want it to include %q", result.StatusMsg, test.want)
			}
		})
	}
}
