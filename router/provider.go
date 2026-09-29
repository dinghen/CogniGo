package router

import (
	providerController "github.com/dinghen/CogniGo/controller/provider"
	"github.com/gin-gonic/gin"
)

func ProviderRouter(r *gin.RouterGroup) {
	r.GET("/providers", providerController.List)
	r.POST("/providers", providerController.Create)
	r.GET("/providers/:id", providerController.Get)
	r.GET("/providers/:id/status", providerController.Get)
	r.PUT("/providers/:id", providerController.Update)
	r.DELETE("/providers/:id", providerController.Delete)
	r.POST("/providers/:id/test", providerController.Test)
	r.POST("/providers/:id/rebuild", providerController.Rebuild)
}
