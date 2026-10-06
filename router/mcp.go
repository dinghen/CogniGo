package router

import (
	mcpController "github.com/dinghen/CogniGo/controller/mcp"
	"github.com/gin-gonic/gin"
)

func MCPRouter(r *gin.RouterGroup) {
	r.GET("/mcp-servers", mcpController.List)
	r.POST("/mcp-servers", mcpController.Create)
	r.GET("/mcp-servers/:id", mcpController.Get)
	r.PUT("/mcp-servers/:id", mcpController.Update)
	r.DELETE("/mcp-servers/:id", mcpController.Delete)
	r.POST("/mcp-servers/:id/test", mcpController.Test)
}
