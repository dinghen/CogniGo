package file

import (
	"github.com/dinghen/CogniGo/common/code"
	"github.com/dinghen/CogniGo/controller"
	"github.com/dinghen/CogniGo/service/file"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type (
	UploadFileResponse struct {
		FilePath string `json:"file_path,omitempty"`
		controller.Response
	}
	ListFileResponse struct {
		controller.Response
		Files []file.FileInfo `json:"files,omitempty"`
	}
)

func UploadRagFile(c *gin.Context) {
	res := new(UploadFileResponse)
	uploadedFile, err := c.FormFile("file")
	if err != nil {
		log.Println("FormFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}

	username := c.GetString("userName")
	if username == "" {
		log.Println("Username not found in context")
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidToken))
		return
	}

	//indexer 会在 service 层根据实际文件名创建
	filePath, err := file.UploadRagFile(username, uploadedFile)
	if err != nil {
		log.Println("UploadFile fail ", err)
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}

	res.Success()
	res.FilePath = filePath
	c.JSON(http.StatusOK, res)
}

func ListRagFiles(c *gin.Context) {
	res := new(ListFileResponse)
	items, err := file.ListRagFiles(c.GetString("userName"))
	if err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}
	res.Success()
	res.Files = items
	c.JSON(http.StatusOK, res)
}

func DeleteRagFile(c *gin.Context) {
	res := new(controller.Response)
	if err := file.DeleteRagFile(c.GetString("userName"), c.Param("name")); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}
	res.Success()
	c.JSON(http.StatusOK, res)
}
