package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/dinghen/CogniGo/common/aihelper"
	"github.com/dinghen/CogniGo/common/code"
	"github.com/dinghen/CogniGo/common/rag"
	messageDAO "github.com/dinghen/CogniGo/dao/message"
	"github.com/dinghen/CogniGo/dao/session"
	"github.com/dinghen/CogniGo/model"
	mcpService "github.com/dinghen/CogniGo/service/mcp"
	providerService "github.com/dinghen/CogniGo/service/provider"
	"log"
	"net/http"

	"github.com/google/uuid"
)

var ctx = context.Background()

// CapabilitySelection is the persisted, user-scoped model selection for one
// session. Nil IDs preserve deployment defaults for legacy requests.
type CapabilitySelection struct {
	ChatProviderID       *uint64
	EmbeddingProviderID  *uint64
	MCPServerIDs         []uint64
	MCPToolIDs           []uint64
	MCPSelectionProvided bool
}

func GetUserSessionsByUserName(userName string) ([]model.SessionInfo, error) {
	//获取用户的所有会话ID

	rows, err := session.GetSessionsByUsername(userName)
	if err != nil {
		return nil, err
	}
	infos := make([]model.SessionInfo, 0, len(rows))
	for i := range rows {
		servers, tools := decodeMCPSelection(&rows[i])
		if servers == nil {
			servers = []uint64{}
		}
		if tools == nil {
			tools = []uint64{}
		}
		infos = append(infos, model.SessionInfo{
			SessionID:           rows[i].ID,
			Title:               rows[i].Title,
			ChatProviderID:      rows[i].ChatProviderID,
			EmbeddingProviderID: rows[i].EmbeddingProviderID,
			MCPServerIDs:        servers,
			MCPToolIDs:          tools,
		})
	}
	return infos, nil
}

func CreateSessionAndSendMessage(userName string, userQuestion string, modelType string, mcpServerIDs, mcpToolIDs []uint64) (string, string, code.Code) {
	return CreateSessionAndSendMessageWithProviders(userName, userQuestion, modelType, mcpServerIDs, mcpToolIDs, CapabilitySelection{})
}

func CreateSessionAndSendMessageWithProviders(userName string, userQuestion string, modelType string, mcpServerIDs, mcpToolIDs []uint64, selection CapabilitySelection) (string, string, code.Code) {
	modelType = normalizeModelType(modelType, selection)
	if err := mcpService.ValidateSelection(userName, mcpServerIDs, mcpToolIDs); err != nil {
		return "", "", code.CodeInvalidParams
	}
	selection.MCPServerIDs = append([]uint64{}, mcpServerIDs...)
	selection.MCPToolIDs = append([]uint64{}, mcpToolIDs...)
	config, err := providerConfig(userName, selection, modelType)
	if err != nil {
		return "", "", code.CodeInvalidParams
	}
	serverJSON, _ := json.Marshal(mcpServerIDs)
	toolJSON, _ := json.Marshal(mcpToolIDs)
	//1：创建一个新的会话
	newSession := &model.Session{
		ID:       uuid.New().String(),
		UserName: userName,
		Title:    userQuestion, // 可以根据需求设置标题，这边暂时用用户第一次的问题作为标题
	}
	newSession.MCPServerIDsJSON = string(serverJSON)
	newSession.MCPToolIDsJSON = string(toolJSON)
	newSession.ChatProviderID = normalizeProviderID(selection.ChatProviderID)
	newSession.EmbeddingProviderID = normalizeProviderID(selection.EmbeddingProviderID)
	createdSession, err := session.CreateSession(newSession)
	if err != nil {
		log.Println("CreateSessionAndSendMessage CreateSession error:", err)
		return "", "", code.CodeServerBusy
	}

	//2：获取AIHelper并通过其管理消息
	manager := aihelper.GetGlobalManager()
	config["username"] = userName
	config["mcpServerIDs"] = mcpServerIDs
	config["mcpToolIDs"] = mcpToolIDs
	helper, err := manager.GetOrCreateAIHelper(userName, createdSession.ID, modelType, config)
	if err != nil {
		log.Println("CreateSessionAndSendMessage GetOrCreateAIHelper error:", err)
		return "", "", modelRuntimeErrorCode(err)
	}

	//3：生成AI回复
	aiResponse, err_ := helper.GenerateResponse(userName, ctx, userQuestion)
	if err_ != nil {
		log.Println("CreateSessionAndSendMessage GenerateResponse error:", err_)
		return "", "", modelRuntimeErrorCode(err_)
	}

	return createdSession.ID, aiResponse.Content, code.CodeSuccess
}

func CreateStreamSessionOnly(userName string, userQuestion string, mcpServerIDs, mcpToolIDs []uint64) (string, code.Code) {
	return CreateStreamSessionOnlyWithProviders(userName, userQuestion, mcpServerIDs, mcpToolIDs, CapabilitySelection{})
}

func CreateStreamSessionOnlyWithProviders(userName string, userQuestion string, mcpServerIDs, mcpToolIDs []uint64, selection CapabilitySelection) (string, code.Code) {
	if err := mcpService.ValidateSelection(userName, mcpServerIDs, mcpToolIDs); err != nil {
		return "", code.CodeInvalidParams
	}
	selection.MCPServerIDs = append([]uint64{}, mcpServerIDs...)
	selection.MCPToolIDs = append([]uint64{}, mcpToolIDs...)
	if _, err := providerService.ResolveSelectedProviders(userName, normalizeProviderID(selection.ChatProviderID), normalizeProviderID(selection.EmbeddingProviderID)); err != nil {
		return "", code.CodeInvalidParams
	}
	serverJSON, _ := json.Marshal(mcpServerIDs)
	toolJSON, _ := json.Marshal(mcpToolIDs)
	newSession := &model.Session{
		ID:                  uuid.New().String(),
		UserName:            userName,
		Title:               userQuestion,
		MCPServerIDsJSON:    string(serverJSON),
		MCPToolIDsJSON:      string(toolJSON),
		ChatProviderID:      normalizeProviderID(selection.ChatProviderID),
		EmbeddingProviderID: normalizeProviderID(selection.EmbeddingProviderID),
	}
	createdSession, err := session.CreateSession(newSession)
	if err != nil {
		log.Println("CreateStreamSessionOnly CreateSession error:", err)
		return "", code.CodeServerBusy
	}
	return createdSession.ID, code.CodeSuccess
}

func StreamMessageToExistingSession(userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	return StreamMessageToExistingSessionWithProviders(userName, sessionID, userQuestion, modelType, writer, CapabilitySelection{})
}

func StreamMessageToExistingSessionWithProviders(userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter, requested CapabilitySelection) code.Code {
	// 确保 writer 支持 Flush
	flusher, ok := writer.(http.Flusher)
	if !ok {
		log.Println("StreamMessageToExistingSession: streaming unsupported")
		return code.CodeServerBusy
	}

	row, config, selection, err := existingSessionConfig(userName, sessionID, requested, modelType)
	if err != nil {
		if errors.Is(err, providerService.ErrInvalidKind) || errors.Is(err, providerService.ErrInvalidInput) {
			return code.CodeInvalidParams
		}
		return code.CodeRecordNotFound
	}
	manager := aihelper.GetGlobalManager()
	config["mcpServerIDs"], config["mcpToolIDs"] = decodeMCPSelection(row)
	modelType = normalizeModelType(modelType, selection)
	helper, err := manager.GetOrCreateAIHelper(userName, sessionID, modelType, config)
	if err != nil {
		log.Println("StreamMessageToExistingSession GetOrCreateAIHelper error:", err)
		return modelRuntimeErrorCode(err)
	}

	cb := func(msg string) {
		// 直接发送数据，不转义
		// SSE 格式：data: <content>\n\n
		log.Printf("[SSE] Sending chunk: %s (len=%d)\n", msg, len(msg))
		_, err := writer.Write([]byte("data: " + msg + "\n\n"))
		if err != nil {
			log.Println("[SSE] Write error:", err)
			return
		}
		flusher.Flush() //  每次必须 flush
		log.Println("[SSE] Flushed")
	}

	_, err_ := helper.StreamResponse(userName, ctx, cb, userQuestion)
	if err_ != nil {
		log.Println("StreamMessageToExistingSession StreamResponse error:", err_)
		return modelRuntimeErrorCode(err_)
	}

	_, err = writer.Write([]byte("data: [DONE]\n\n"))
	if err != nil {
		log.Println("StreamMessageToExistingSession write DONE error:", err)
		return code.AIModelFail
	}
	flusher.Flush()

	return code.CodeSuccess
}

func CreateStreamSessionAndSendMessage(userName string, userQuestion string, modelType string, writer http.ResponseWriter, mcpServerIDs, mcpToolIDs []uint64) (string, code.Code) {
	return CreateStreamSessionAndSendMessageWithProviders(userName, userQuestion, modelType, writer, mcpServerIDs, mcpToolIDs, CapabilitySelection{})
}

func CreateStreamSessionAndSendMessageWithProviders(userName string, userQuestion string, modelType string, writer http.ResponseWriter, mcpServerIDs, mcpToolIDs []uint64, selection CapabilitySelection) (string, code.Code) {

	sessionID, code_ := CreateStreamSessionOnlyWithProviders(userName, userQuestion, mcpServerIDs, mcpToolIDs, selection)
	if code_ != code.CodeSuccess {
		return "", code_
	}

	code_ = StreamMessageToExistingSessionWithProviders(userName, sessionID, userQuestion, modelType, writer, CapabilitySelection{})
	if code_ != code.CodeSuccess {

		return sessionID, code_
	}

	return sessionID, code.CodeSuccess
}

func ChatSend(userName string, sessionID string, userQuestion string, modelType string) (string, code.Code) {
	return ChatSendWithProviders(userName, sessionID, userQuestion, modelType, CapabilitySelection{})
}

func ChatSendWithProviders(userName string, sessionID string, userQuestion string, modelType string, requested CapabilitySelection) (string, code.Code) {
	//1：获取AIHelper
	manager := aihelper.GetGlobalManager()
	row, config, selection, err := existingSessionConfig(userName, sessionID, requested, modelType)
	if err != nil {
		if errors.Is(err, providerService.ErrInvalidKind) || errors.Is(err, providerService.ErrInvalidInput) {
			return "", code.CodeInvalidParams
		}
		return "", code.CodeRecordNotFound
	}
	config["mcpServerIDs"], config["mcpToolIDs"] = decodeMCPSelection(row)
	modelType = normalizeModelType(modelType, selection)
	helper, err := manager.GetOrCreateAIHelper(userName, sessionID, modelType, config)
	if err != nil {
		log.Println("ChatSend GetOrCreateAIHelper error:", err)
		return "", modelRuntimeErrorCode(err)
	}

	//2：生成AI回复
	aiResponse, err_ := helper.GenerateResponse(userName, ctx, userQuestion)
	if err_ != nil {
		log.Println("ChatSend GenerateResponse error:", err_)
		return "", modelRuntimeErrorCode(err_)
	}

	return aiResponse.Content, code.CodeSuccess
}

func GetChatHistory(userName string, sessionID string) ([]model.History, code.Code) {
	if _, err := session.GetSessionByIDForUser(userName, sessionID); err != nil {
		return nil, code.CodeRecordNotFound
	}
	manager := aihelper.GetGlobalManager()
	if helper, exists := manager.GetAIHelper(userName, sessionID); exists {
		messages := helper.GetMessages()
		history := make([]model.History, 0, len(messages))
		for _, msg := range messages {
			history = append(history, model.History{IsUser: msg.IsUser, Content: msg.Content})
		}
		return history, code.CodeSuccess
	}
	messages, err := messageDAO.GetMessagesBySessionIDForUser(userName, sessionID)
	if err != nil {
		return nil, code.CodeServerBusy
	}
	history := make([]model.History, 0, len(messages))
	for _, msg := range messages {
		history = append(history, model.History{IsUser: msg.IsUser, Content: msg.Content})
	}
	return history, code.CodeSuccess
}

func ChatStreamSend(userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter) code.Code {
	return ChatStreamSendWithProviders(userName, sessionID, userQuestion, modelType, writer, CapabilitySelection{})
}

func ChatStreamSendWithProviders(userName string, sessionID string, userQuestion string, modelType string, writer http.ResponseWriter, requested CapabilitySelection) code.Code {
	return StreamMessageToExistingSessionWithProviders(userName, sessionID, userQuestion, modelType, writer, requested)
}

func decodeMCPSelection(row *model.Session) ([]uint64, []uint64) {
	if row == nil {
		return nil, nil
	}
	var servers, tools []uint64
	_ = json.Unmarshal([]byte(row.MCPServerIDsJSON), &servers)
	_ = json.Unmarshal([]byte(row.MCPToolIDsJSON), &tools)
	return servers, tools
}

func providerConfig(userName string, selection CapabilitySelection, modelType string) (map[string]interface{}, error) {
	resolved, err := providerService.ResolveSelectedProviders(userName, normalizeProviderID(selection.ChatProviderID), normalizeProviderID(selection.EmbeddingProviderID))
	if err != nil {
		return nil, err
	}
	if modelType == "capability" {
		if resolved.Chat == nil {
			fallback, err := providerService.Resolve(userName, "chat")
			if err != nil {
				return nil, err
			}
			resolved.Chat = &fallback
		}
		if resolved.Embedding == nil {
			fallback, err := providerService.Resolve(userName, "embedding")
			if err != nil {
				return nil, err
			}
			resolved.Embedding = &fallback
		}
	}
	config := map[string]interface{}{"username": userName}
	if resolved.Chat != nil {
		config["chatProvider"] = *resolved.Chat
	}
	if resolved.Embedding != nil {
		config["embeddingProvider"] = *resolved.Embedding
	}
	config["mcpServerIDs"] = selection.MCPServerIDs
	config["mcpToolIDs"] = selection.MCPToolIDs
	runtimeSelections, err := mcpService.ResolveRuntimeSelections(userName, selection.MCPServerIDs, selection.MCPToolIDs)
	if err != nil {
		return nil, err
	}
	serialized, err := json.Marshal(runtimeSelections)
	if err != nil {
		return nil, err
	}
	mcpFingerprint := sha256.Sum256(serialized)
	config["mcpFingerprint"] = hex.EncodeToString(mcpFingerprint[:])
	return config, nil
}

func existingSessionConfig(userName, sessionID string, requested CapabilitySelection, modelType string) (*model.Session, map[string]interface{}, CapabilitySelection, error) {
	row, err := session.GetSessionByIDForUser(userName, sessionID)
	if err != nil {
		return nil, nil, CapabilitySelection{}, err
	}
	servers, tools := decodeMCPSelection(row)
	selection := CapabilitySelection{
		ChatProviderID: row.ChatProviderID, EmbeddingProviderID: row.EmbeddingProviderID,
		MCPServerIDs: servers, MCPToolIDs: tools,
	}
	modelType = normalizeModelType(modelType, selection)
	capabilitiesChanged := false
	if requested.ChatProviderID != nil {
		selection.ChatProviderID = normalizeProviderID(requested.ChatProviderID)
		capabilitiesChanged = true
	}
	if requested.EmbeddingProviderID != nil {
		selection.EmbeddingProviderID = normalizeProviderID(requested.EmbeddingProviderID)
		capabilitiesChanged = true
	}
	var serverJSON, toolJSON *string
	if requested.MCPSelectionProvided {
		if err := mcpService.ValidateSelection(userName, requested.MCPServerIDs, requested.MCPToolIDs); err != nil {
			return nil, nil, CapabilitySelection{}, err
		}
		selection.MCPServerIDs = append([]uint64{}, requested.MCPServerIDs...)
		selection.MCPToolIDs = append([]uint64{}, requested.MCPToolIDs...)
		serverBytes, _ := json.Marshal(selection.MCPServerIDs)
		toolBytes, _ := json.Marshal(selection.MCPToolIDs)
		serverText, toolText := string(serverBytes), string(toolBytes)
		serverJSON, toolJSON = &serverText, &toolText
		capabilitiesChanged = true
	}
	var config map[string]interface{}
	if capabilitiesChanged {
		config, err = providerConfig(userName, selection, modelType)
		if err != nil {
			return nil, nil, CapabilitySelection{}, err
		}
		if err := session.UpdateCapabilitySelections(userName, sessionID, selection.ChatProviderID, selection.EmbeddingProviderID, serverJSON, toolJSON); err != nil {
			return nil, nil, CapabilitySelection{}, err
		}
		row.ChatProviderID, row.EmbeddingProviderID = selection.ChatProviderID, selection.EmbeddingProviderID
		if serverJSON != nil {
			row.MCPServerIDsJSON, row.MCPToolIDsJSON = *serverJSON, *toolJSON
		}
	} else {
		config, err = providerConfig(userName, selection, modelType)
		if err != nil {
			return nil, nil, CapabilitySelection{}, err
		}
	}
	return row, config, selection, nil
}

func normalizeModelType(modelType string, selection CapabilitySelection) string {
	modelType = strings.TrimSpace(modelType)
	if modelType != "" {
		return modelType
	}
	return "capability"
}

func normalizeProviderID(id *uint64) *uint64 {
	if id == nil || *id == 0 {
		return nil
	}
	value := *id
	return &value
}

func modelRuntimeErrorCode(err error) code.Code {
	if errors.Is(err, rag.ErrIndexRebuildRequired) {
		return code.AIModelRebuildRequired
	}
	return code.AIModelFail
}
