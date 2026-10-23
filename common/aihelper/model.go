package aihelper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	mcpclient "github.com/dinghen/CogniGo/common/mcp/client"
	"github.com/dinghen/CogniGo/common/rag"
	"github.com/dinghen/CogniGo/config"
	mcpService "github.com/dinghen/CogniGo/service/mcp"
	providerService "github.com/dinghen/CogniGo/service/provider"
)

type StreamCallback func(msg string)

// AIModel 定义AI模型接口
type AIModel interface {
	GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error)
	StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error)
	GetModelType() string
}

// =================== OpenAI 实现 ===================
type OpenAIModel struct {
	llm model.ToolCallingChatModel
}

func NewOpenAIModel(ctx context.Context) (*OpenAIModel, error) {
	return newOpenAIModel(ctx, os.Getenv("OPENAI_BASE_URL"), os.Getenv("OPENAI_MODEL_NAME"), os.Getenv("OPENAI_API_KEY"))
}

func NewOpenAIModelForUser(ctx context.Context, username string) (*OpenAIModel, error) {
	resolved, err := providerService.Resolve(username, "chat")
	if err != nil {
		return nil, fmt.Errorf("resolve chat provider: %w", err)
	}
	return newOpenAIModel(ctx, resolved.BaseURL, resolved.Model, resolved.APIKey)
}

// NewOpenAIModelForProvider constructs a chat model from a provider already
// resolved by the authenticated session boundary.
func NewOpenAIModelForProvider(ctx context.Context, resolved providerService.ResolvedConfig) (*OpenAIModel, error) {
	return newOpenAIModel(ctx, resolved.BaseURL, resolved.Model, resolved.APIKey)
}

func newOpenAIModel(ctx context.Context, baseURL, modelName, key string) (*OpenAIModel, error) {

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
		APIKey:  key,
	})
	if err != nil {
		return nil, fmt.Errorf("create openai model failed: %w", err)
	}
	return &OpenAIModel{llm: llm}, nil
}

func (o *OpenAIModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	resp, err := o.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("openai generate failed: %w", err)
	}
	return resp, nil
}

func (o *OpenAIModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("openai stream failed: %w", err)
	}
	defer stream.Close()

	var fullResp strings.Builder

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("openai stream recv failed: %w", err)
		}
		if len(msg.Content) > 0 {
			fullResp.WriteString(msg.Content) // 聚合

			cb(msg.Content) // 实时调用cb函数，方便主动发送给前端
		}
	}

	return fullResp.String(), nil //返回完整内容，方便后续存储
}

func (o *OpenAIModel) GetModelType() string { return "1" }

// =================== Ollama 实现 ===================

// OllamaModel Ollama模型实现
type OllamaModel struct {
	llm model.ToolCallingChatModel
}

func NewOllamaModel(ctx context.Context, baseURL, modelName string) (*OllamaModel, error) {
	llm, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
	})
	if err != nil {
		return nil, fmt.Errorf("create ollama model failed: %w", err)
	}
	return &OllamaModel{llm: llm}, nil
}

func (o *OllamaModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	resp, err := o.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("ollama generate failed: %w", err)
	}
	return resp, nil
}

func (o *OllamaModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("ollama stream failed: %w", err)
	}
	defer stream.Close()
	var fullResp strings.Builder
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("ollama stream recv failed: %w", err)
		}
		if len(msg.Content) > 0 {
			fullResp.WriteString(msg.Content) // 聚合
			cb(msg.Content)                   // 实时调用cb函数，方便主动发送给前端
		}
	}
	return fullResp.String(), nil //返回完整内容，方便后续存储
}

func (o *OllamaModel) GetModelType() string { return "4" }

// =================== RAG 实现 ===================
type AliRAGModel struct {
	llm       model.ToolCallingChatModel
	username  string // 用于获取用户的文档
	embedding providerService.ResolvedConfig
}

func NewAliRAGModel(ctx context.Context, username string) (*AliRAGModel, error) {
	resolved, err := providerService.Resolve(username, "chat")
	if err != nil {
		return nil, fmt.Errorf("resolve chat provider: %w", err)
	}
	if !resolved.FromUser {
		conf := config.GetConfig()
		resolved.BaseURL = conf.RagModelConfig.RagBaseUrl
		resolved.Model = conf.RagModelConfig.RagChatModelName
	}
	embedding, err := providerService.Resolve(username, "embedding")
	if err != nil {
		return nil, fmt.Errorf("resolve embedding provider: %w", err)
	}
	return newAliRAGModel(ctx, username, resolved, embedding)
}

// NewAliRAGModelForProviders threads independently selected chat and embedding
// capabilities through the RAG model. Nil values retain deployment fallback.
func NewAliRAGModelForProviders(ctx context.Context, username string, chat, embedding *providerService.ResolvedConfig) (*AliRAGModel, error) {
	if chat == nil {
		resolved, err := providerService.Resolve(username, "chat")
		if err != nil {
			return nil, fmt.Errorf("resolve chat provider: %w", err)
		}
		chat = &resolved
		if !chat.FromUser {
			conf := config.GetConfig()
			chat.BaseURL = conf.RagModelConfig.RagBaseUrl
			chat.Model = conf.RagModelConfig.RagChatModelName
		}
	}
	if embedding == nil {
		resolved, err := providerService.Resolve(username, "embedding")
		if err != nil {
			return nil, fmt.Errorf("resolve embedding provider: %w", err)
		}
		embedding = &resolved
	}
	return newAliRAGModel(ctx, username, *chat, *embedding)
}

func newAliRAGModel(ctx context.Context, username string, chat, embedding providerService.ResolvedConfig) (*AliRAGModel, error) {

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: chat.BaseURL,
		Model:   chat.Model,
		APIKey:  chat.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("create ali rag model failed: %w", err)
	}
	return &AliRAGModel{
		llm:       llm,
		username:  username,
		embedding: embedding,
	}, nil
}

func (o *AliRAGModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	query := messages[len(messages)-1].Content

	// 1. 创建 RAG 查询器
	ragQuery, err := rag.NewRAGQueryForProvider(ctx, o.username, o.embedding)
	if err != nil {
		if shouldFallbackToPlainChat(err) {
			return o.llm.Generate(ctx, messages)
		}
		return nil, fmt.Errorf("prepare RAG query: %w", err)
	}

	// 2. 检索相关文档
	docs, err := ragQuery.RetrieveDocuments(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("retrieve RAG documents: %w", err)
	}

	// 3. 构建包含检索结果的提示词并调用 LLM。
	resp, err := o.llm.Generate(ctx, withRAGPrompt(messages, query, docs))
	if err != nil {
		return nil, fmt.Errorf("ali rag generate failed: %w", err)
	}
	return resp, nil
}

func (o *AliRAGModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}
	query := messages[len(messages)-1].Content

	// 1. 创建 RAG 查询器
	ragQuery, err := rag.NewRAGQueryForProvider(ctx, o.username, o.embedding)
	if err != nil {
		if shouldFallbackToPlainChat(err) {
			return o.streamMessages(ctx, messages, cb)
		}
		return "", fmt.Errorf("prepare RAG query: %w", err)
	}

	// 2. 检索相关文档
	docs, err := ragQuery.RetrieveDocuments(ctx, query)
	if err != nil {
		return "", fmt.Errorf("retrieve RAG documents: %w", err)
	}

	return o.streamWithRAGPrompt(ctx, messages, query, docs, cb)
}

func withRAGPrompt(messages []*schema.Message, query string, docs []*schema.Document) []*schema.Message {
	return rag.BuildRAGMessages(messages, query, docs)
}

func shouldFallbackToPlainChat(err error) bool {
	return errors.Is(err, rag.ErrNoKnowledgeFiles)
}

func (o *AliRAGModel) streamWithRAGPrompt(ctx context.Context, messages []*schema.Message, query string, docs []*schema.Document, cb StreamCallback) (string, error) {
	return o.streamMessages(ctx, withRAGPrompt(messages, query, docs), cb)
}

func (o *AliRAGModel) streamMessages(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("ali rag stream failed: %w", err)
	}
	defer stream.Close()

	var fullResp strings.Builder

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("ali rag stream recv failed: %w", err)
		}
		if len(msg.Content) > 0 {
			fullResp.WriteString(msg.Content)
			if cb != nil {
				cb(msg.Content)
			}
		}
	}

	return fullResp.String(), nil
}

func (o *AliRAGModel) GetModelType() string { return "2" }

// =================== MCP 实现 ===================

// MCPModel MCP模型实现，集成MCP服务
type MCPModel struct {
	llm           model.ToolCallingChatModel
	username      string
	embedding     *providerService.ResolvedConfig
	mcpClients    []*mcpclient.MCPClient
	mcpBaseURL    string
	mcpSelections []mcpService.RuntimeSelection
	modelType     string
	mu            sync.Mutex
}

// NewMCPModel 创建MCP模型实例
func NewMCPModel(ctx context.Context, username string) (*MCPModel, error) {
	return NewMCPModelWithSelections(ctx, username, nil, nil)
}

func NewMCPModelWithSelections(ctx context.Context, username string, serverIDs, toolIDs []uint64) (*MCPModel, error) {
	resolved, err := providerService.Resolve(username, "chat")
	if err != nil {
		return nil, fmt.Errorf("resolve chat provider for mcp: %w", err)
	}

	return newMCPModelWithProvider(ctx, username, resolved, serverIDs, toolIDs)
}

// NewMCPModelWithProvider uses the selected chat provider while retaining the
// existing ownership-checked MCP selection flow.
func NewMCPModelWithProvider(ctx context.Context, username string, resolved providerService.ResolvedConfig, serverIDs, toolIDs []uint64) (*MCPModel, error) {
	return newMCPModelWithProviders(ctx, username, &resolved, nil, serverIDs, toolIDs)
}

func newMCPModelWithProvider(ctx context.Context, username string, resolved providerService.ResolvedConfig, serverIDs, toolIDs []uint64) (*MCPModel, error) {
	return newMCPModelWithProviders(ctx, username, &resolved, nil, serverIDs, toolIDs)
}

// NewMCPModelWithProviders creates the unified tool-calling runtime. A
// selected embedding provider adds the user's RAG context before the native
// MCP agent runs; a nil embedding provider leaves ordinary tool chat intact.
func NewMCPModelWithProviders(ctx context.Context, username string, chat, embedding *providerService.ResolvedConfig, serverIDs, toolIDs []uint64) (*MCPModel, error) {
	if chat == nil {
		resolved, err := providerService.Resolve(username, "chat")
		if err != nil {
			return nil, fmt.Errorf("resolve chat provider for capability model: %w", err)
		}
		chat = &resolved
	}
	return newMCPModelWithProviders(ctx, username, chat, embedding, serverIDs, toolIDs)
}

func newMCPModelWithProviders(ctx context.Context, username string, chat, embedding *providerService.ResolvedConfig, serverIDs, toolIDs []uint64) (*MCPModel, error) {
	// 创建LLM
	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: chat.BaseURL,
		Model:   chat.Model,
		APIKey:  chat.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("create mcp model failed: %w", err)
	}

	mcpBaseURL := config.GetConfig().RuntimeConfig.MCPURL
	if value := os.Getenv("COGNIGO_MCP_URL"); value != "" {
		mcpBaseURL = value
	}

	selections, err := mcpService.ResolveRuntimeSelections(username, serverIDs, toolIDs)
	if err != nil {
		return nil, fmt.Errorf("resolve MCP session selection: %w", err)
	}
	return &MCPModel{llm: llm, username: username, embedding: embedding, mcpBaseURL: mcpBaseURL, mcpSelections: selections, modelType: "3"}, nil
}

// NewCapabilityModel is the unified runtime used by the new chat workspace.
// It combines the selected knowledge retriever with zero or more MCP tools.
func NewCapabilityModel(ctx context.Context, username string, chat, embedding *providerService.ResolvedConfig, serverIDs, toolIDs []uint64) (*MCPModel, error) {
	if chat == nil {
		resolved, err := providerService.Resolve(username, "chat")
		if err != nil {
			return nil, fmt.Errorf("resolve chat provider for capability model: %w", err)
		}
		chat = &resolved
	}
	capabilityModel, err := newMCPModelWithProviders(ctx, username, chat, embedding, serverIDs, toolIDs)
	if err != nil {
		return nil, err
	}
	capabilityModel.modelType = "capability"
	return capabilityModel, nil
}

func (m *MCPModel) getMCPClients(ctx context.Context) ([]*mcpclient.MCPClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.mcpClients) > 0 {
		return m.mcpClients, nil
	}
	if len(m.mcpSelections) == 0 {
		return nil, fmt.Errorf("no MCP servers selected for this session")
	}
	clients := make([]*mcpclient.MCPClient, 0, len(m.mcpSelections))
	for _, selection := range m.mcpSelections {
		client, err := mcpclient.NewMCPClientWithConfig(ctx, selection.Config)
		if err != nil {
			for _, connected := range clients {
				_ = connected.Close()
			}
			return nil, err
		}
		clients = append(clients, client)
	}
	m.mcpClients = clients
	return clients, nil
}

func (m *MCPModel) einoTools(ctx context.Context) ([]tool.BaseTool, error) {
	if len(m.mcpSelections) == 0 {
		return nil, nil
	}
	clients, err := m.getMCPClients(ctx)
	if err != nil {
		return nil, err
	}
	var out []tool.BaseTool
	for i, client := range clients {
		var allowed map[string]struct{}
		if i < len(m.mcpSelections) {
			allowed = m.mcpSelections[i].AllowedTools
		}
		tools, err := client.EinoToolsAllowed(ctx, allowed)
		if err != nil {
			return nil, err
		}
		out = append(out, tools...)
	}
	return out, nil
}

// GenerateResponse 生成响应，集成MCP工具
func (m *MCPModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	messages, err := m.withRAGContext(ctx, messages)
	if err != nil {
		return nil, err
	}
	tools, err := m.einoTools(ctx)
	if err != nil {
		return nil, err
	}
	if len(tools) == 0 {
		return m.llm.Generate(ctx, messages)
	}
	mcpAgent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: m.llm,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: tools},
		MaxStep:          m.maxSteps(),
	})
	if err != nil {
		return nil, fmt.Errorf("create MCP agent: %w", err)
	}
	return mcpAgent.Generate(ctx, messages, agent.WithComposeOptions(compose.WithRuntimeMaxSteps(m.maxSteps())))
}

// StreamResponse uses the same native ReAct graph as GenerateResponse.
func (m *MCPModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}
	messages, err := m.withRAGContext(ctx, messages)
	if err != nil {
		return "", err
	}
	tools, err := m.einoTools(ctx)
	if err != nil {
		return "", err
	}
	if len(tools) == 0 {
		stream, err := m.llm.Stream(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("capability stream failed: %w", err)
		}
		defer stream.Close()
		var response strings.Builder
		for {
			msg, recvErr := stream.Recv()
			if recvErr == io.EOF {
				break
			}
			if recvErr != nil {
				return "", fmt.Errorf("receive capability stream: %w", recvErr)
			}
			if msg != nil && msg.Content != "" {
				response.WriteString(msg.Content)
				if cb != nil {
					cb(msg.Content)
				}
			}
		}
		return response.String(), nil
	}
	mcpAgent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: m.llm,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: tools},
		MaxStep:          m.maxSteps(),
	})
	if err != nil {
		return "", fmt.Errorf("create MCP agent: %w", err)
	}
	stream, err := mcpAgent.Stream(ctx, messages, agent.WithComposeOptions(compose.WithRuntimeMaxSteps(m.maxSteps())))
	if err != nil {
		return "", fmt.Errorf("stream MCP agent: %w", err)
	}
	defer stream.Close()
	var response strings.Builder
	for {
		msg, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			return "", fmt.Errorf("receive MCP agent stream: %w", recvErr)
		}
		if msg != nil && msg.Content != "" {
			response.WriteString(msg.Content)
			if cb != nil {
				cb(msg.Content)
			}
		}
	}
	return response.String(), nil
}

func (m *MCPModel) withRAGContext(ctx context.Context, messages []*schema.Message) ([]*schema.Message, error) {
	if m.embedding == nil || len(messages) == 0 {
		return messages, nil
	}
	query := messages[len(messages)-1].Content
	queryEngine, err := rag.NewRAGQueryForProvider(ctx, m.username, *m.embedding)
	if err != nil {
		if errors.Is(err, rag.ErrNoKnowledgeFiles) {
			return messages, nil
		}
		return nil, fmt.Errorf("prepare knowledge retrieval: %w", err)
	}
	docs, err := queryEngine.RetrieveDocuments(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("retrieve knowledge context: %w", err)
	}
	return withRAGPrompt(messages, query, docs), nil
}

func (m *MCPModel) maxSteps() int {
	steps := config.GetConfig().RuntimeConfig.MCPMaxSteps
	if steps <= 0 {
		steps = 12
	}
	if value, err := strconv.Atoi(os.Getenv("COGNIGO_MCP_MAX_STEPS")); err == nil && value > 0 {
		steps = value
	}
	return steps
}

// GetModelType returns the MCP model identifier.
func (m *MCPModel) GetModelType() string {
	if m.modelType != "" {
		return m.modelType
	}
	return "3"
}

// Close closes the official MCP session.
func (m *MCPModel) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, client := range m.mcpClients {
		_ = client.Close()
	}
	m.mcpClients = nil
}
