package router

import (
	"github.com/dinghen/CogniGo/controller/file"

	"github.com/gin-gonic/gin"
)

func FileRouter(r *gin.RouterGroup) {
	r.POST("/upload", file.UploadRagFile)
	r.GET("/files", file.ListRagFiles)
	r.DELETE("/files/:name", file.DeleteRagFile)
}
