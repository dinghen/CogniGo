package aihelper

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	mcpclient "github.com/dinghen/CogniGo/common/mcp/client"
	"github.com/dinghen/CogniGo/common/rag"
	"github.com/dinghen/CogniGo/config"
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
	key := os.Getenv("OPENAI_API_KEY")
	modelName := os.Getenv("OPENAI_MODEL_NAME")
	baseURL := os.Getenv("OPENAI_BASE_URL")

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
		APIKey:  key,
	})
	if err != nil {
		return nil, fmt.Errorf("create openai model failed: %v", err)
	}
	return &OpenAIModel{llm: llm}, nil
}

func (o *OpenAIModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	resp, err := o.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("openai generate failed: %v", err)
	}
	return resp, nil
}

func (o *OpenAIModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("openai stream failed: %v", err)
	}
	defer stream.Close()

	var fullResp strings.Builder

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("openai stream recv failed: %v", err)
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
		return nil, fmt.Errorf("create ollama model failed: %v", err)
	}
	return &OllamaModel{llm: llm}, nil
}

func (o *OllamaModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	resp, err := o.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("ollama generate failed: %v", err)
	}
	return resp, nil
}

func (o *OllamaModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("ollama stream failed: %v", err)
	}
	defer stream.Close()
	var fullResp strings.Builder
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("openai stream recv failed: %v", err)
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
	llm      model.ToolCallingChatModel
	username string // 用于获取用户的文档
}

func NewAliRAGModel(ctx context.Context, username string) (*AliRAGModel, error) {
	key := os.Getenv("OPENAI_API_KEY")
	conf := config.GetConfig()
	modelName := conf.RagModelConfig.RagChatModelName
	baseURL := conf.RagModelConfig.RagBaseUrl

	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
		APIKey:  key,
	})
	if err != nil {
		return nil, fmt.Errorf("create ali rag model failed: %v", err)
	}
	return &AliRAGModel{
		llm:      llm,
		username: username,
	}, nil
}

func (o *AliRAGModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	query := messages[len(messages)-1].Content

	// 1. 创建 RAG 查询器
	ragQuery, err := rag.NewRAGQuery(ctx, o.username)
	if err != nil {
		log.Printf("Failed to create RAG query (user may not have uploaded file): %v", err)
		resp, err := o.llm.Generate(ctx, withRAGPrompt(messages, query, nil))
		if err != nil {
			return nil, fmt.Errorf("ali rag generate failed: %v", err)
		}
		return resp, nil
	}

	// 2. 检索相关文档
	docs, err := ragQuery.RetrieveDocuments(ctx, query)
	if err != nil {
		log.Printf("Failed to retrieve documents: %v", err)
		resp, err := o.llm.Generate(ctx, withRAGPrompt(messages, query, nil))
		if err != nil {
			return nil, fmt.Errorf("ali rag generate failed: %v", err)
		}
		return resp, nil
	}

	// 3. 构建包含检索结果的提示词并调用 LLM。
	resp, err := o.llm.Generate(ctx, withRAGPrompt(messages, query, docs))
	if err != nil {
		return nil, fmt.Errorf("ali rag generate failed: %v", err)
	}
	return resp, nil
}

func (o *AliRAGModel) StreamResponse(ctx context.Context, messages []*schema.Message, cb StreamCallback) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}
	query := messages[len(messages)-1].Content

	// 1. 创建 RAG 查询器
	ragQuery, err := rag.NewRAGQuery(ctx, o.username)
	if err != nil {
		log.Printf("Failed to create RAG query (user may not have uploaded file): %v", err)
		return o.streamWithRAGPrompt(ctx, messages, query, nil, cb)
	}

	// 2. 检索相关文档
	docs, err := ragQuery.RetrieveDocuments(ctx, query)
	if err != nil {
		log.Printf("Failed to retrieve documents: %v", err)
		return o.streamWithRAGPrompt(ctx, messages, query, nil, cb)
	}

	return o.streamWithRAGPrompt(ctx, messages, query, docs, cb)
}

func withRAGPrompt(messages []*schema.Message, query string, docs []*schema.Document) []*schema.Message {
	ragMessages := make([]*schema.Message, len(messages))
	copy(ragMessages, messages)
	ragMessages[len(ragMessages)-1] = &schema.Message{Role: schema.User, Content: rag.BuildRAGPrompt(query, docs)}
	return ragMessages
}

func (o *AliRAGModel) streamWithRAGPrompt(ctx context.Context, messages []*schema.Message, query string, docs []*schema.Document, cb StreamCallback) (string, error) {
	stream, err := o.llm.Stream(ctx, withRAGPrompt(messages, query, docs))
	if err != nil {
		return "", fmt.Errorf("ali rag stream failed: %v", err)
	}
	defer stream.Close()

	var fullResp strings.Builder

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("ali rag stream recv failed: %v", err)
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
	llm        model.ToolCallingChatModel
	mcpClient  *mcpclient.MCPClient
	mcpBaseURL string
	mu         sync.Mutex
}

// NewMCPModel 创建MCP模型实例
func NewMCPModel(ctx context.Context, username string) (*MCPModel, error) {
	key := os.Getenv("OPENAI_API_KEY")
	conf := config.GetConfig()
	modelName := conf.RagModelConfig.RagChatModelName
	baseURL := conf.RagModelConfig.RagBaseUrl

	// 创建LLM
	llm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL: baseURL,
		Model:   modelName,
		APIKey:  key,
	})
	if err != nil {
		return nil, fmt.Errorf("create mcp model failed: %v", err)
	}

	mcpBaseURL := config.GetConfig().RuntimeConfig.MCPURL
	if value := os.Getenv("COGNIGO_MCP_URL"); value != "" {
		mcpBaseURL = value
	}

	return &MCPModel{llm: llm, mcpBaseURL: mcpBaseURL}, nil
}

func (m *MCPModel) getMCPClient(ctx context.Context) (*mcpclient.MCPClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mcpClient != nil {
		return m.mcpClient, nil
	}
	client, err := mcpclient.NewMCPClient(ctx, m.mcpBaseURL)
	if err != nil {
		return nil, err
	}
	m.mcpClient = client
	return client, nil
}

// GenerateResponse 生成响应，集成MCP工具
func (m *MCPModel) GenerateResponse(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	client, err := m.getMCPClient(ctx)
	if err != nil {
		return nil, err
	}
	tools, err := client.EinoTools(ctx)
	if err != nil {
		return nil, err
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
	client, err := m.getMCPClient(ctx)
	if err != nil {
		return "", err
	}
	tools, err := client.EinoTools(ctx)
	if err != nil {
		return "", err
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
func (m *MCPModel) GetModelType() string { return "3" }

// Close closes the official MCP session.
func (m *MCPModel) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mcpClient != nil {
		_ = m.mcpClient.Close()
		m.mcpClient = nil
	}
}
