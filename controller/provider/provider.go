package provider

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/dinghen/CogniGo/common/code"
	"github.com/dinghen/CogniGo/common/rag"
	"github.com/dinghen/CogniGo/controller"
	providerService "github.com/dinghen/CogniGo/service/provider"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type response struct {
	controller.Response
	Provider  providerService.ProviderDTO   `json:"provider,omitempty"`
	Items     []providerService.ProviderDTO `json:"providers,omitempty"`
	Dimension int                           `json:"dimension,omitempty"`
}

var errInvalidProviderID = errors.New("invalid provider id")

func List(c *gin.Context) {
	res := new(response)
	items, err := providerService.List(c.GetString("userName"))
	if err != nil {
		writeError(c, res, err)
		return
	}
	res.Success()
	res.Items = items
	c.JSON(http.StatusOK, res)
}

func Get(c *gin.Context) {
	res := new(response)
	id, err := providerID(c)
	if err == nil {
		var item providerService.ProviderDTO
		item, err = providerService.Get(c.GetString("userName"), id)
		if err == nil {
			res.Success()
			res.Provider = item
			c.JSON(http.StatusOK, res)
			return
		}
	}
	writeError(c, res, err)
}

func Create(c *gin.Context) {
	res := new(response)
	var input providerService.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, res, providerService.ErrInvalidInput)
		return
	}
	item, err := providerService.Create(c.GetString("userName"), input)
	if err != nil {
		writeError(c, res, err)
		return
	}
	res.Success()
	res.Provider = item
	c.JSON(http.StatusCreated, res)
}

func Update(c *gin.Context) {
	res := new(response)
	id, err := providerID(c)
	if err != nil {
		writeError(c, res, err)
		return
	}
	var input providerService.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, res, providerService.ErrInvalidInput)
		return
	}
	item, err := providerService.Update(c.GetString("userName"), id, input)
	if err != nil {
		writeError(c, res, err)
		return
	}
	res.Success()
	res.Provider = item
	c.JSON(http.StatusOK, res)
}

func Delete(c *gin.Context) {
	res := new(response)
	id, err := providerID(c)
	if err == nil {
		err = providerService.Delete(c.GetString("userName"), id)
	}
	if err != nil {
		writeError(c, res, err)
		return
	}
	res.Success()
	c.JSON(http.StatusOK, res)
}

func Test(c *gin.Context) {
	res := new(response)
	id, err := providerID(c)
	if err != nil {
		writeError(c, res, err)
		return
	}
	item, err := providerService.Get(c.GetString("userName"), id)
	if err != nil {
		writeError(c, res, err)
		return
	}
	resolved, err := providerService.ResolveByID(c.GetString("userName"), id)
	if err != nil {
		writeError(c, res, err)
		return
	}
	if item.Kind == "embedding" {
		dimension, probeErr := rag.ProbeEmbeddingConfig(context.Background(), resolved.BaseURL, resolved.Model, resolved.APIKey)
		if probeErr != nil {
			writeError(c, res, probeErr)
			return
		}
		item, err = providerService.MarkEmbeddingProbe(c.GetString("userName"), id, dimension)
		if err != nil {
			writeError(c, res, err)
			return
		}
		res.Dimension = dimension
	} else if err := providerService.TestChat(context.Background(), resolved); err != nil {
		writeError(c, res, err)
		return
	}
	res.Success()
	res.Provider = item
	c.JSON(http.StatusOK, res)
}

func Rebuild(c *gin.Context) {
	res := new(response)
	id, err := providerID(c)
	if err != nil {
		writeError(c, res, err)
		return
	}
	item, err := providerService.Get(c.GetString("userName"), id)
	if err != nil {
		writeError(c, res, err)
		return
	}
	if item.Kind != "embedding" {
		writeError(c, res, providerService.ErrInvalidKind)
		return
	}
	resolved, err := providerService.ResolveByID(c.GetString("userName"), id)
	if err != nil {
		writeError(c, res, err)
		return
	}
	dimension, err := rag.RebuildIndexForUserProvider(c.Request.Context(), c.GetString("userName"), resolved)
	if err != nil {
		writeError(c, res, err)
		return
	}
	item, err = providerService.MarkEmbeddingRebuilt(c.GetString("userName"), id, dimension)
	if err != nil {
		writeError(c, res, err)
		return
	}
	res.Success()
	res.Dimension = dimension
	res.Provider = item
	c.JSON(http.StatusOK, res)
}

func providerID(c *gin.Context) (uint64, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id == 0 {
		return 0, errInvalidProviderID
	}
	return id, nil
}

func writeError(c *gin.Context, res *response, err error) {
	if err == nil {
		err = errors.New("provider operation failed")
	}
	status, responseCode, message := safeProviderError(err)
	// Do not return upstream provider bodies: they can contain credentials or
	// vendor-specific request details. Log only the error type for diagnosis.
	log.Printf("provider operation failed: %T", err)
	res.StatusCode = responseCode
	res.StatusMsg = message
	c.JSON(status, res)
}

func safeProviderError(err error) (int, code.Code, string) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return http.StatusNotFound, code.CodeRecordNotFound, "provider not found"
	case errors.Is(err, providerService.ErrInvalidKind):
		return http.StatusBadRequest, code.CodeInvalidParams, "provider kind must be chat or embedding"
	case errors.Is(err, providerService.ErrInvalidInput):
		return http.StatusBadRequest, code.CodeInvalidParams, "provider parameters are invalid"
	case errors.Is(err, errInvalidProviderID):
		return http.StatusBadRequest, code.CodeInvalidParams, "invalid provider id"
	case errors.Is(err, providerService.ErrEncryptionKey):
		return http.StatusServiceUnavailable, code.CodeServerBusy, "provider encryption is not configured"
	case errors.Is(err, providerService.ErrSecretUnavailable):
		return http.StatusServiceUnavailable, code.CodeServerBusy, "provider credential is unavailable"
	default:
		return http.StatusServiceUnavailable, code.CodeServerBusy, "provider service unavailable"
	}
}
