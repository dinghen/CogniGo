package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	einoJSONSchema "github.com/eino-contrib/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPClient owns one initialized official MCP client session.
type MCPClient struct {
	session *sdk.ClientSession
}

// NewMCPClient connects and performs the official Initialize handshake.
func NewMCPClient(ctx context.Context, httpURL string) (*MCPClient, error) {
	client := sdk.NewClient(&sdk.Implementation{Name: "cognigo", Version: "1.0.0"}, &sdk.ClientOptions{
		Capabilities: &sdk.ClientCapabilities{},
		// Registering the handler enables the SDK's subscriptions/listen stream;
		// the SDK invalidates its ListTools cache when a list-changed notification arrives.
		ToolListChangedHandler: func(context.Context, *sdk.ToolListChangedRequest) {},
	})
	transport := &sdk.StreamableClientTransport{Endpoint: httpURL}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect MCP server: %w", err)
	}
	return &MCPClient{session: session}, nil
}

func (m *MCPClient) Session() *sdk.ClientSession { return m.session }

func (m *MCPClient) InitializeResult() *sdk.InitializeResult {
	if m == nil || m.session == nil {
		return nil
	}
	return m.session.InitializeResult()
}

func (m *MCPClient) Ping(ctx context.Context) error {
	if m == nil || m.session == nil {
		return fmt.Errorf("MCP client is not initialized")
	}
	return m.session.Ping(ctx, nil)
}

func (m *MCPClient) ListTools(ctx context.Context) ([]*sdk.Tool, error) {
	if m == nil || m.session == nil {
		return nil, fmt.Errorf("MCP client is not initialized")
	}
	var tools []*sdk.Tool
	for item, err := range m.session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list MCP tools: %w", err)
		}
		tools = append(tools, item)
	}
	return tools, nil
}

func (m *MCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*sdk.CallToolResult, error) {
	if m == nil || m.session == nil {
		return nil, fmt.Errorf("MCP client is not initialized")
	}
	result, err := m.session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("call MCP tool %q: %w", name, err)
	}
	return result, nil
}

func (m *MCPClient) Close() error {
	if m == nil || m.session == nil {
		return nil
	}
	return m.session.Close()
}

// EinoTools discovers all server tools and adapts them to Eino's native tool interface.
func (m *MCPClient) EinoTools(ctx context.Context) ([]tool.BaseTool, error) {
	tools, err := m.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tool.BaseTool, 0, len(tools))
	for _, item := range tools {
		adapted, err := NewEinoTool(m, item)
		if err != nil {
			return nil, fmt.Errorf("adapt MCP tool %q: %w", item.Name, err)
		}
		out = append(out, adapted)
	}
	return out, nil
}

// EinoTool adapts one official MCP Tool to an Eino InvokableTool.
type EinoTool struct {
	client *MCPClient
	info   *schema.ToolInfo
}

func NewEinoTool(client *MCPClient, item *sdk.Tool) (*EinoTool, error) {
	if item == nil || item.Name == "" {
		return nil, fmt.Errorf("MCP tool has no name")
	}
	params, err := schemaFromMCP(item.InputSchema)
	if err != nil {
		return nil, err
	}
	return &EinoTool{client: client, info: &schema.ToolInfo{Name: item.Name, Desc: item.Description, ParamsOneOf: params}}, nil
}

func (t *EinoTool) Info(context.Context) (*schema.ToolInfo, error) { return t.info, nil }

func (t *EinoTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	args := map[string]any{}
	if argumentsInJSON != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			return "", fmt.Errorf("decode arguments for %q: %w", t.info.Name, err)
		}
	}
	result, err := t.client.CallTool(ctx, t.info.Name, args)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(struct {
		Content           []sdk.Content `json:"content,omitempty"`
		StructuredContent any           `json:"structuredContent,omitempty"`
		IsError           bool          `json:"isError,omitempty"`
	}{Content: result.Content, StructuredContent: result.StructuredContent, IsError: result.IsError})
	if err != nil {
		return "", fmt.Errorf("encode result for %q: %w", t.info.Name, err)
	}
	// Tool execution failures are protocol-level successful responses with
	// IsError=true. Preserve that flag in the tool message so the model can
	// inspect and recover; transport/protocol failures still return an error.
	return string(encoded), nil
}

func schemaFromMCP(raw any) (*schema.ParamsOneOf, error) {
	if raw == nil {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal input schema: %w", err)
	}
	var jsonSchema einoJSONSchema.Schema
	if err := json.Unmarshal(data, &jsonSchema); err != nil {
		return nil, fmt.Errorf("decode input schema: %w", err)
	}
	return schema.NewParamsOneOfByJSONSchema(&jsonSchema), nil
}

var _ tool.InvokableTool = (*EinoTool)(nil)
