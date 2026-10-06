package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	einoJSONSchema "github.com/eino-contrib/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPClient owns one initialized official MCP client session.
type MCPClient struct {
	session *sdk.ClientSession
}

// Config describes a user-approved MCP transport. It intentionally contains
// plaintext secrets only for the lifetime of a request.
type Config struct {
	Transport        string
	URL              string
	Headers          map[string]string
	Command          string
	Args             []string
	Env              map[string]string
	AllowPrivateHTTP bool
}

// Selection is the user-authorized runtime view of one MCP server.
type Selection struct {
	ServerID     uint64
	Config       Config
	AllowedTools map[string]struct{}
}

// NewMCPClient connects and performs the official Initialize handshake.
func NewMCPClient(ctx context.Context, httpURL string) (*MCPClient, error) {
	// The fixed demo URL may point at a local in-process test server. User-owned
	// configurations use NewMCPClientWithConfig with the SSRF policy applied by
	// the service layer.
	return NewMCPClientWithConfig(ctx, Config{Transport: "streamable-http", URL: httpURL, AllowPrivateHTTP: true})
}

// NewMCPClientWithConfig constructs an official SDK client for either
// Streamable HTTP or stdio. Commands are passed directly to exec.Command.
func NewMCPClientWithConfig(ctx context.Context, cfg Config) (*MCPClient, error) {
	client := sdk.NewClient(&sdk.Implementation{Name: "cognigo", Version: "1.0.0"}, &sdk.ClientOptions{
		Capabilities: &sdk.ClientCapabilities{},
		// Registering the handler enables the SDK's subscriptions/listen stream;
		// the SDK invalidates its ListTools cache when a list-changed notification arrives.
		ToolListChangedHandler: func(context.Context, *sdk.ToolListChangedRequest) {},
	})
	var transport sdk.Transport
	switch strings.ToLower(strings.TrimSpace(cfg.Transport)) {
	case "", "streamable-http", "http":
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, fmt.Errorf("MCP HTTP URL is required")
		}
		transport = &sdk.StreamableClientTransport{Endpoint: cfg.URL, HTTPClient: headerClient(cfg.Headers, cfg.AllowPrivateHTTP), MaxRetries: 1}
	case "stdio":
		if strings.TrimSpace(cfg.Command) == "" {
			return nil, fmt.Errorf("MCP command is required")
		}
		cmd := exec.Command(cfg.Command, cfg.Args...)
		cmd.Env = append(os.Environ(), mapEnv(cfg.Env)...)
		transport = &sdk.CommandTransport{Command: cmd}
	default:
		return nil, fmt.Errorf("unsupported MCP transport %q", cfg.Transport)
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect MCP server: %w", err)
	}
	return &MCPClient{session: session}, nil
}

func mapEnv(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

func headerClient(headers map[string]string, allowPrivate bool) *http.Client {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, item := range ips {
			ip := item.IP
			if !allowPrivate && !isPublicAddress(ip) {
				lastErr = fmt.Errorf("MCP target resolves to a non-public address")
				continue
			}
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("MCP target resolved to no addresses")
		}
		return nil, lastErr
	}
	return &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		for key, value := range headers {
			clone.Header.Set(key, value)
		}
		return base.RoundTrip(clone)
	}), CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && !sameOrigin(req.URL, via[len(via)-1].URL) {
			return fmt.Errorf("MCP redirect to another origin is not allowed")
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many MCP redirects")
		}
		return nil
	}}
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
func isPublicAddress(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func (m *MCPClient) Session() *sdk.ClientSession {
	if m == nil {
		return nil
	}
	return m.session
}

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
	return m.EinoToolsAllowed(ctx, nil)
}

// EinoToolsAllowed discovers tools and optionally limits the result to the
// names selected for the current session. A nil allowlist means all tools.
func (m *MCPClient) EinoToolsAllowed(ctx context.Context, allowed map[string]struct{}) ([]tool.BaseTool, error) {
	tools, err := m.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]tool.BaseTool, 0, len(tools))
	for _, item := range tools {
		if allowed != nil {
			if _, ok := allowed[item.Name]; !ok {
				continue
			}
		}
		adapted, err := NewEinoTool(m, item)
		if err != nil {
			name := "<unknown>"
			if item != nil {
				name = item.Name
			}
			return nil, fmt.Errorf("adapt MCP tool %q: %w", name, err)
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
	if client == nil || client.session == nil {
		return nil, fmt.Errorf("MCP client is not initialized")
	}
	if item == nil || item.Name == "" {
		return nil, fmt.Errorf("MCP tool has no name")
	}
	params, err := schemaFromMCP(item.InputSchema)
	if err != nil {
		return nil, err
	}
	return &EinoTool{client: client, info: &schema.ToolInfo{Name: item.Name, Desc: item.Description, ParamsOneOf: params}}, nil
}

func (t *EinoTool) Info(context.Context) (*schema.ToolInfo, error) {
	if t == nil || t.info == nil {
		return nil, fmt.Errorf("MCP tool is not initialized")
	}
	return t.info, nil
}

func (t *EinoTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	if t == nil || t.client == nil || t.client.session == nil {
		return "", fmt.Errorf("MCP tool is not initialized")
	}
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
	if result == nil {
		return "", fmt.Errorf("MCP tool %q returned an empty result", t.info.Name)
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
