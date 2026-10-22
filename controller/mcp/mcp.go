package mcp

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/dinghen/CogniGo/common/code"
	"github.com/dinghen/CogniGo/controller"
	mcpService "github.com/dinghen/CogniGo/service/mcp"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var errInvalidMCPID = errors.New("invalid MCP server id")

type response struct {
	controller.Response
	Server  mcpService.DTO   `json:"server,omitempty"`
	Servers []mcpService.DTO `json:"servers,omitempty"`
}

func List(c *gin.Context) {
	res := new(response)
	items, err := mcpService.List(c.GetString("userName"))
	if err != nil {
		returnError(c, res, err)
		return
	}
	res.Success()
	res.Servers = items
	c.JSON(http.StatusOK, res)
}
func Get(c *gin.Context) {
	res := new(response)
	id, err := parseID(c)
	if err == nil {
		var item mcpService.DTO
		item, err = mcpService.Get(c.GetString("userName"), id)
		if err == nil {
			res.Success()
			res.Server = item
			c.JSON(http.StatusOK, res)
			return
		}
	}
	returnError(c, res, err)
}
func Create(c *gin.Context) {
	res := new(response)
	var in mcpService.Input
	if err := bindInput(c, &in); err != nil {
		returnError(c, res, err)
		return
	}
	item, err := mcpService.Create(c.GetString("userName"), in)
	if err != nil {
		returnError(c, res, err)
		return
	}
	res.Success()
	res.Server = item
	c.JSON(http.StatusCreated, res)
}
func Update(c *gin.Context) {
	res := new(response)
	id, err := parseID(c)
	if err != nil {
		returnError(c, res, err)
		return
	}
	var in mcpService.Input
	if err := bindInput(c, &in); err != nil {
		returnError(c, res, err)
		return
	}
	item, err := mcpService.Update(c.GetString("userName"), id, in)
	if err != nil {
		returnError(c, res, err)
		return
	}
	res.Success()
	res.Server = item
	c.JSON(http.StatusOK, res)
}
func Delete(c *gin.Context) {
	res := new(response)
	id, err := parseID(c)
	if err == nil {
		err = mcpService.Delete(c.GetString("userName"), id)
	}
	if err != nil {
		returnError(c, res, err)
		return
	}
	res.Success()
	c.JSON(http.StatusOK, res)
}
func Test(c *gin.Context) {
	res := new(response)
	id, err := parseID(c)
	if err == nil {
		var item mcpService.DTO
		item, err = mcpService.Test(c.Request.Context(), c.GetString("userName"), id)
		if err == nil {
			res.Success()
			res.Server = item
			c.JSON(http.StatusOK, res)
			return
		}
	}
	returnError(c, res, err)
}
func parseID(c *gin.Context) (uint64, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		return 0, errInvalidMCPID
	}
	return id, nil
}

func bindInput(c *gin.Context, in *mcpService.Input) error {
	if err := c.ShouldBindJSON(in); err != nil {
		if errors.Is(err, mcpService.ErrInvalidInput) || errors.Is(err, mcpService.ErrInvalidHeaders) || errors.Is(err, mcpService.ErrInvalidEnv) {
			return err
		}
		return fmt.Errorf("%w: %v", mcpService.ErrInvalidInput, err)
	}
	return nil
}

func returnError(c *gin.Context, res *response, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, res.CodeOf(code.CodeRecordNotFound))
		return
	}
	if errors.Is(err, mcpService.ErrUnavailable) {
		res.StatusCode = code.CodeServerBusy
		res.StatusMsg = "MCP service unavailable"
		c.JSON(http.StatusServiceUnavailable, res)
		return
	}
	if errors.Is(err, errInvalidMCPID) || errors.Is(err, mcpService.ErrInvalidInput) ||
		errors.Is(err, mcpService.ErrInvalidHeaders) || errors.Is(err, mcpService.ErrInvalidEnv) ||
		errors.Is(err, mcpService.ErrInvalidTransport) || errors.Is(err, mcpService.ErrPrivateHTTP) {
		res.StatusCode = code.CodeInvalidParams
		res.StatusMsg = err.Error()
		c.JSON(http.StatusBadRequest, res)
		return
	}
	res.StatusCode = code.CodeServerBusy
	res.StatusMsg = "MCP service unavailable"
	c.JSON(http.StatusServiceUnavailable, res)
}
