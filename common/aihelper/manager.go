package aihelper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	providerService "github.com/dinghen/CogniGo/service/provider"
)

var ctx = context.Background()

// AIHelperManager AI助手管理器，管理用户-会话-AIHelper的映射关系
type AIHelperManager struct {
	helpers map[string]map[string]*AIHelper // map[用户账号（唯一）]map[会话ID]*AIHelper
	mu      sync.RWMutex
}

// NewAIHelperManager 创建新的管理器实例
func NewAIHelperManager() *AIHelperManager {
	return &AIHelperManager{
		helpers: make(map[string]map[string]*AIHelper),
	}
}

// 获取或创建AIHelper
func (m *AIHelperManager) GetOrCreateAIHelper(userName string, sessionID string, modelType string, config map[string]interface{}) (*AIHelper, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 获取用户的会话映射
	userHelpers, exists := m.helpers[userName]
	if !exists {
		userHelpers = make(map[string]*AIHelper)
		m.helpers[userName] = userHelpers
	}

	// 检查会话是否已存在
	helper, exists := userHelpers[sessionID]
	if exists {
		fingerprint := capabilityFingerprint(modelType, config)
		if helper.configFingerprint != fingerprint {
			factory := GetGlobalFactory()
			model, err := factory.CreateAIModel(ctx, modelType, config)
			if err != nil {
				return nil, err
			}
			helper.replaceModel(model, fingerprint)
		}
		return helper, nil
	}

	// 创建新的AIHelper
	factory := GetGlobalFactory()
	helper, err := factory.CreateAIHelper(ctx, modelType, sessionID, config)
	if err != nil {
		return nil, err
	}
	helper.configFingerprint = capabilityFingerprint(modelType, config)

	userHelpers[sessionID] = helper
	return helper, nil
}

func capabilityFingerprint(modelType string, config map[string]interface{}) string {
	chat, _ := config["chatProvider"].(providerService.ResolvedConfig)
	embedding, _ := config["embeddingProvider"].(providerService.ResolvedConfig)
	chatKeyFingerprint := sha256.Sum256([]byte(chat.APIKey))
	embeddingKeyFingerprint := sha256.Sum256([]byte(embedding.APIKey))
	servers, _ := config["mcpServerIDs"].([]uint64)
	tools, _ := config["mcpToolIDs"].([]uint64)
	mcpFingerprint, _ := config["mcpFingerprint"].(string)
	serialized := fmt.Sprintf("%s|chat:%s:%s:%s:%s:%x|embedding:%s:%s:%s:%s:%d:%x|servers:%v|tools:%v|mcp:%s",
		modelType, chat.Name, chat.Kind, chat.BaseURL, chat.Model, chatKeyFingerprint, embedding.Name, embedding.Kind, embedding.BaseURL, embedding.Model, embedding.Dimension, embeddingKeyFingerprint, servers, tools, mcpFingerprint)
	fingerprint := sha256.Sum256([]byte(serialized))
	return hex.EncodeToString(fingerprint[:])
}

// 获取指定用户的指定会话的AIHelper
func (m *AIHelperManager) GetAIHelper(userName string, sessionID string) (*AIHelper, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return nil, false
	}

	helper, exists := userHelpers[sessionID]
	return helper, exists
}

// 移除指定用户的指定会话的AIHelper
func (m *AIHelperManager) RemoveAIHelper(userName string, sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return
	}

	delete(userHelpers, sessionID)

	// 如果用户没有会话了，清理用户映射
	if len(userHelpers) == 0 {
		delete(m.helpers, userName)
	}
}

// 获取指定用户的所有会话ID
func (m *AIHelperManager) GetUserSessions(userName string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	userHelpers, exists := m.helpers[userName]
	if !exists {
		return []string{}
	}

	sessionIDs := make([]string, 0, len(userHelpers))
	//取出所有的key
	for sessionID := range userHelpers {
		sessionIDs = append(sessionIDs, sessionID)
	}

	return sessionIDs
}

// 全局管理器实例
var globalManager *AIHelperManager
var once sync.Once

// GetGlobalManager 获取全局管理器实例
func GetGlobalManager() *AIHelperManager {
	once.Do(func() {
		globalManager = NewAIHelperManager()
	})
	return globalManager
}
