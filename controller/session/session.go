package session

import (
	"fmt"
	"github.com/dinghen/CogniGo/common/code"
	"github.com/dinghen/CogniGo/controller"
	"github.com/dinghen/CogniGo/model"
	"github.com/dinghen/CogniGo/service/session"
	"net/http"

	"github.com/gin-gonic/gin"
)

type (
	GetUserSessionsResponse struct {
		controller.Response
		Sessions []model.SessionInfo `json:"sessions,omitempty"`
	}
	CreateSessionAndSendMessageRequest struct {
		UserQuestion        string   `json:"question" binding:"required"` // 用户问题;
		ModelType           string   `json:"modelType,omitempty"`         // legacy model selector;
		ChatProviderID      *uint64  `json:"chatProviderId,omitempty"`
		EmbeddingProviderID *uint64  `json:"embeddingProviderId,omitempty"`
		MCPServerIDs        []uint64 `json:"mcpServerIds,omitempty"`
		MCPToolIDs          []uint64 `json:"mcpToolIds,omitempty"`
	}

	CreateSessionAndSendMessageResponse struct {
		AiInformation string `json:"Information,omitempty"` // AI回答
		SessionID     string `json:"sessionId,omitempty"`   // 当前会话ID
		controller.Response
	}

	ChatSendRequest struct {
		UserQuestion        string   `json:"question" binding:"required"`            // 用户问题;
		ModelType           string   `json:"modelType,omitempty"`                    // legacy model selector;
		SessionID           string   `json:"sessionId,omitempty" binding:"required"` // 当前会话ID
		ChatProviderID      *uint64  `json:"chatProviderId,omitempty"`
		EmbeddingProviderID *uint64  `json:"embeddingProviderId,omitempty"`
		MCPServerIDs        []uint64 `json:"mcpServerIds,omitempty"`
		MCPToolIDs          []uint64 `json:"mcpToolIds,omitempty"`
	}

	ChatSendResponse struct {
		AiInformation string `json:"Information,omitempty"` // AI回答
		controller.Response
	}

	ChatHistoryRequest struct {
		SessionID string `json:"sessionId,omitempty" binding:"required"` // 当前会话ID
	}
	ChatHistoryResponse struct {
		History []model.History `json:"history"`
		controller.Response
	}
)

func GetUserSessionsByUserName(c *gin.Context) {
	res := new(GetUserSessionsResponse)
	userName := c.GetString("userName") // From JWT middleware

	userSessions, err := session.GetUserSessionsByUserName(userName)
	if err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeServerBusy))
		return
	}

	res.Success()
	res.Sessions = userSessions
	c.JSON(http.StatusOK, res)
}

func CreateSessionAndSendMessage(c *gin.Context) {
	req := new(CreateSessionAndSendMessageRequest)
	res := new(CreateSessionAndSendMessageResponse)
	userName := c.GetString("userName") // From JWT middleware
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}
	//内部会创建会话并发送消息，并会将AI回答、当前会话返回
	selection := session.CapabilitySelection{ChatProviderID: req.ChatProviderID, EmbeddingProviderID: req.EmbeddingProviderID, MCPServerIDs: req.MCPServerIDs, MCPToolIDs: req.MCPToolIDs, MCPSelectionProvided: req.MCPServerIDs != nil || req.MCPToolIDs != nil}
	session_id, aiInformation, code_ := session.CreateSessionAndSendMessageWithProviders(userName, req.UserQuestion, req.ModelType, req.MCPServerIDs, req.MCPToolIDs, selection)

	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.AiInformation = aiInformation
	res.SessionID = session_id
	c.JSON(http.StatusOK, res)
}

func CreateStreamSessionAndSendMessage(c *gin.Context) {
	req := new(CreateSessionAndSendMessageRequest)
	userName := c.GetString("userName") // From JWT middleware
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, gin.H{"error": "Invalid parameters"})
		return
	}

	// 设置SSE头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no") // 禁止代理缓存

	// 先创建会话并立即把 sessionId 下发给前端，随后再开始流式输出
	selection := session.CapabilitySelection{ChatProviderID: req.ChatProviderID, EmbeddingProviderID: req.EmbeddingProviderID, MCPServerIDs: req.MCPServerIDs, MCPToolIDs: req.MCPToolIDs, MCPSelectionProvided: req.MCPServerIDs != nil || req.MCPToolIDs != nil}
	sessionID, code_ := session.CreateStreamSessionOnlyWithProviders(userName, req.UserQuestion, req.MCPServerIDs, req.MCPToolIDs, selection)
	if code_ != code.CodeSuccess {
		c.SSEvent("error", gin.H{"status_code": code_.Code(), "status_msg": code_.Msg()})
		return
	}

	// 先把 sessionId 通过 data 事件发送给前端，前端据此绑定当前会话，侧边栏即可出现新标签
	c.Writer.WriteString(fmt.Sprintf("data: {\"sessionId\": \"%s\"}\n\n", sessionID))
	c.Writer.Flush()

	// 然后开始把本次回答进行流式发送（包含最后的 [DONE]）
	code_ = session.StreamMessageToExistingSessionWithProviders(userName, sessionID, req.UserQuestion, req.ModelType, http.ResponseWriter(c.Writer), selection)
	if code_ != code.CodeSuccess {
		c.SSEvent("error", gin.H{"status_code": code_.Code(), "status_msg": code_.Msg()})
		return
	}
}

func ChatSend(c *gin.Context) {
	req := new(ChatSendRequest)
	res := new(ChatSendResponse)
	userName := c.GetString("userName") // From JWT middleware
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}
	// 发送消息，并会将AI回答返回
	selection := session.CapabilitySelection{ChatProviderID: req.ChatProviderID, EmbeddingProviderID: req.EmbeddingProviderID, MCPServerIDs: req.MCPServerIDs, MCPToolIDs: req.MCPToolIDs, MCPSelectionProvided: req.MCPServerIDs != nil || req.MCPToolIDs != nil}
	aiInformation, code_ := session.ChatSendWithProviders(userName, req.SessionID, req.UserQuestion, req.ModelType, selection)

	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.AiInformation = aiInformation
	c.JSON(http.StatusOK, res)
}

func ChatStreamSend(c *gin.Context) {
	req := new(ChatSendRequest)
	userName := c.GetString("userName") // From JWT middleware
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, gin.H{"error": "Invalid parameters"})
		return
	}

	// 设置SSE头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no") // 禁止代理缓存

	selection := session.CapabilitySelection{ChatProviderID: req.ChatProviderID, EmbeddingProviderID: req.EmbeddingProviderID, MCPServerIDs: req.MCPServerIDs, MCPToolIDs: req.MCPToolIDs, MCPSelectionProvided: req.MCPServerIDs != nil || req.MCPToolIDs != nil}
	code_ := session.ChatStreamSendWithProviders(userName, req.SessionID, req.UserQuestion, req.ModelType, http.ResponseWriter(c.Writer), selection)
	if code_ != code.CodeSuccess {
		c.SSEvent("error", gin.H{"status_code": code_.Code(), "status_msg": code_.Msg()})
		return
	}

}

func ChatHistory(c *gin.Context) {
	req := new(ChatHistoryRequest)
	res := new(ChatHistoryResponse)
	userName := c.GetString("userName") // From JWT middleware
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(http.StatusOK, res.CodeOf(code.CodeInvalidParams))
		return
	}
	history, code_ := session.GetChatHistory(userName, req.SessionID)
	if code_ != code.CodeSuccess {
		c.JSON(http.StatusOK, res.CodeOf(code_))
		return
	}

	res.Success()
	res.History = history
	c.JSON(http.StatusOK, res)
}
