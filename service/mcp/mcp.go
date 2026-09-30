package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcpclient "github.com/dinghen/CogniGo/common/mcp/client"
	mcpDAO "github.com/dinghen/CogniGo/dao/mcp"
	"github.com/dinghen/CogniGo/dao/user"
	"github.com/dinghen/CogniGo/model"
	providerService "github.com/dinghen/CogniGo/service/provider"
)

var (
	ErrInvalidInput     = errors.New("MCP configuration is invalid")
	ErrInvalidTransport = errors.New("MCP transport must be streamable-http or stdio")
	ErrPrivateHTTP      = errors.New("private MCP HTTP targets are not allowed")
)

type Input struct {
	Name      string            `json:"name"`
	Transport string            `json:"transport"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	Enabled   *bool             `json:"enabled"`
}

type DTO struct {
	ID            uint64            `json:"id"`
	Name          string            `json:"name"`
	Transport     string            `json:"transport"`
	URL           string            `json:"url,omitempty"`
	HeadersMasked map[string]string `json:"headers,omitempty"`
	Command       string            `json:"command,omitempty"`
	Args          []string          `json:"args,omitempty"`
	EnvMasked     map[string]string `json:"env,omitempty"`
	Enabled       bool              `json:"enabled"`
	Status        string            `json:"status"`
	LastTestedAt  *time.Time        `json:"last_tested_at,omitempty"`
	Tools         []ToolDTO         `json:"tools,omitempty"`
}
type ToolDTO struct {
	ID          uint64          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	Enabled     bool            `json:"enabled"`
}

// RuntimeSelection is the decrypted, user-authorized MCP configuration used
// only while constructing a session model. Secrets never cross the API DTO.
type RuntimeSelection struct {
	ServerID     uint64
	Config       mcpclient.Config
	AllowedTools map[string]struct{}
}

func uid(username string) (int64, error) {
	u, err := user.FindByUsername(username)
	if err != nil {
		return 0, err
	}
	if u == nil {
		return 0, mcpDAO.ErrNotFound
	}
	return u.ID, nil
}

func validate(in Input) error {
	if len(strings.TrimSpace(in.Name)) == 0 || len(in.Name) > 100 {
		return ErrInvalidInput
	}
	in.Transport = strings.ToLower(strings.TrimSpace(in.Transport))
	if in.Transport == "http" {
		in.Transport = "streamable-http"
	}
	if in.Transport != "streamable-http" && in.Transport != "stdio" {
		return ErrInvalidTransport
	}
	if in.Transport == "streamable-http" {
		u, err := url.Parse(strings.TrimSpace(in.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return ErrInvalidInput
		}
		if !allowPrivate() {
			host := u.Hostname()
			if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
				return ErrPrivateHTTP
			}
			if strings.EqualFold(host, "localhost") {
				return ErrPrivateHTTP
			}
		}
		if len(in.URL) > 1000 {
			return ErrInvalidInput
		}
	} else {
		if !allowStdio(in.Command) {
			return ErrInvalidInput
		}
		if strings.TrimSpace(in.Command) == "" || strings.ContainsAny(in.Command, "\r\n\t ;&|`$()") || len(in.Args) > 32 {
			return ErrInvalidInput
		}
		for _, arg := range in.Args {
			if len(arg) > 512 {
				return ErrInvalidInput
			}
		}
	}
	if len(in.Headers) > 32 || len(in.Env) > 32 {
		return ErrInvalidInput
	}
	return nil
}

func allowStdio(command string) bool {
	if !strings.EqualFold(os.Getenv("COGNIGO_MCP_ALLOW_STDIO"), "true") {
		return false
	}
	name := filepath.Base(strings.TrimSpace(command))
	allowed := strings.FieldsFunc(os.Getenv("COGNIGO_MCP_STDIO_COMMANDS"), func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	for _, item := range allowed {
		if filepath.Base(strings.TrimSpace(item)) == name {
			return true
		}
	}
	return false
}

func allowPrivate() bool {
	return strings.EqualFold(os.Getenv("COGNIGO_MCP_ALLOW_PRIVATE_HTTP"), "true")
}
func seal(values map[string]string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return providerService.EncryptSecret(string(raw))
}
func open(encoded string) (map[string]string, error) {
	if encoded == "" {
		return map[string]string{}, nil
	}
	raw, err := providerService.DecryptSecret(encoded)
	if err != nil {
		return nil, err
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}
func mask(values map[string]string) map[string]string {
	out := map[string]string{}
	for k := range values {
		out[k] = "********"
	}
	return out
}
func argsJSON(args []string) string { raw, _ := json.Marshal(args); return string(raw) }
func decodeArgs(raw string) []string {
	var args []string
	_ = json.Unmarshal([]byte(raw), &args)
	return args
}

func toDTO(row *model.MCPServer, tools []model.MCPTool, headers, env map[string]string) DTO {
	out := DTO{ID: row.ID, Name: row.Name, Transport: row.Transport, URL: row.URL, Command: row.Command, Args: decodeArgs(row.ArgsJSON), Enabled: row.Enabled, Status: row.Status, LastTestedAt: row.LastTestedAt, HeadersMasked: mask(headers), EnvMasked: mask(env)}
	for _, t := range tools {
		var schema json.RawMessage
		if t.InputSchemaJSON != "" {
			schema = json.RawMessage(t.InputSchemaJSON)
		}
		out.Tools = append(out.Tools, ToolDTO{ID: t.ID, Name: t.Name, Description: t.Description, InputSchema: schema, Enabled: t.Enabled})
	}
	return out
}

func List(username string) ([]DTO, error) {
	id, err := uid(username)
	if err != nil {
		return nil, err
	}
	rows, err := mcpDAO.ListServers(id)
	if err != nil {
		return nil, err
	}
	out := make([]DTO, 0, len(rows))
	for i := range rows {
		h, e := open(rows[i].EncryptedHeaders)
		if e != nil {
			return nil, e
		}
		env, e := open(rows[i].EncryptedEnv)
		if e != nil {
			return nil, e
		}
		ts, _ := mcpDAO.ListTools(rows[i].ID)
		out = append(out, toDTO(&rows[i], ts, h, env))
	}
	return out, nil
}
func Get(username string, id uint64) (DTO, error) {
	uid, err := uid(username)
	if err != nil {
		return DTO{}, err
	}
	row, err := mcpDAO.GetServer(uid, id)
	if err != nil {
		return DTO{}, mcpDAO.ErrNotFound
	}
	h, e := open(row.EncryptedHeaders)
	if e != nil {
		return DTO{}, e
	}
	env, e := open(row.EncryptedEnv)
	if e != nil {
		return DTO{}, e
	}
	ts, e := mcpDAO.ListTools(row.ID)
	if e != nil {
		return DTO{}, e
	}
	return toDTO(row, ts, h, env), nil
}

func Create(username string, in Input) (DTO, error)            { return save(username, 0, in) }
func Update(username string, id uint64, in Input) (DTO, error) { return save(username, id, in) }
func save(username string, id uint64, in Input) (DTO, error) {
	if err := validate(in); err != nil {
		return DTO{}, err
	}
	uid, err := uid(username)
	if err != nil {
		return DTO{}, err
	}
	h, e := seal(in.Headers)
	if e != nil {
		return DTO{}, e
	}
	env, e := seal(in.Env)
	if e != nil {
		return DTO{}, e
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	row := &model.MCPServer{UserID: uid, Name: strings.TrimSpace(in.Name), Transport: strings.ToLower(strings.TrimSpace(in.Transport)), URL: strings.TrimRight(strings.TrimSpace(in.URL), "/"), EncryptedHeaders: h, Command: strings.TrimSpace(in.Command), ArgsJSON: argsJSON(in.Args), EncryptedEnv: env, Enabled: enabled, Status: "unverified"}
	if row.Transport == "http" {
		row.Transport = "streamable-http"
	}
	if id == 0 {
		if err := mcpDAO.CreateServer(row); err != nil {
			return DTO{}, err
		}
	} else {
		existing, e := mcpDAO.GetServer(uid, id)
		if e != nil {
			return DTO{}, mcpDAO.ErrNotFound
		}
		row.ID = id
		row.Status = existing.Status
		if err := mcpDAO.UpdateServer(row); err != nil {
			return DTO{}, err
		}
	}
	return Get(username, row.ID)
}
func Delete(username string, id uint64) error {
	uid, err := uid(username)
	if err != nil {
		return err
	}
	return mcpDAO.DeleteServer(uid, id)
}

// ValidateSelection verifies that selected MCP records belong to the user.
func ValidateSelection(username string, serverIDs, toolIDs []uint64) error {
	userID, err := uid(username)
	if err != nil {
		return err
	}
	servers := make(map[uint64]bool, len(serverIDs))
	for _, id := range serverIDs {
		if id == 0 {
			return ErrInvalidInput
		}
		row, getErr := mcpDAO.GetServer(userID, id)
		if getErr != nil || !row.Enabled {
			return mcpDAO.ErrNotFound
		}
		servers[id] = true
	}
	for _, id := range toolIDs {
		if id == 0 {
			return ErrInvalidInput
		}
		found := false
		for serverID := range servers {
			tool, getErr := mcpDAO.GetTool(serverID, id)
			if getErr == nil && tool.Enabled {
				found = true
				break
			}
		}
		if !found {
			return mcpDAO.ErrNotFound
		}
	}
	return nil
}

// ResolveRuntimeSelections loads only the MCP servers and tools selected for a
// session. An empty selection returns no clients, so ordinary chat sessions do
// not contact any MCP endpoint.
func ResolveRuntimeSelections(username string, serverIDs, toolIDs []uint64) ([]RuntimeSelection, error) {
	if err := ValidateSelection(username, serverIDs, toolIDs); err != nil {
		return nil, err
	}
	if len(serverIDs) == 0 {
		return nil, nil
	}
	userID, err := uid(username)
	if err != nil {
		return nil, err
	}
	selectedTools := make(map[uint64]struct{}, len(toolIDs))
	for _, id := range toolIDs {
		selectedTools[id] = struct{}{}
	}
	selections := make([]RuntimeSelection, 0, len(serverIDs))
	for _, serverID := range serverIDs {
		row, err := mcpDAO.GetServer(userID, serverID)
		if err != nil {
			return nil, mcpDAO.ErrNotFound
		}
		headers, err := open(row.EncryptedHeaders)
		if err != nil {
			return nil, err
		}
		env, err := open(row.EncryptedEnv)
		if err != nil {
			return nil, err
		}
		allowed := map[string]struct{}(nil)
		if len(selectedTools) > 0 {
			allowed = make(map[string]struct{})
			tools, err := mcpDAO.ListTools(row.ID)
			if err != nil {
				return nil, err
			}
			for _, tool := range tools {
				if _, ok := selectedTools[tool.ID]; ok {
					allowed[tool.Name] = struct{}{}
				}
			}
		}
		selections = append(selections, RuntimeSelection{
			ServerID: row.ID,
			Config: mcpclient.Config{
				Transport: row.Transport,
				URL:       row.URL,
				Headers:   headers,
				Command:   row.Command,
				Args:      decodeArgs(row.ArgsJSON),
				Env:       env,
			},
			AllowedTools: allowed,
		})
	}
	return selections, nil
}

func Test(ctx context.Context, username string, id uint64) (DTO, error) {
	uid, err := uid(username)
	if err != nil {
		return DTO{}, err
	}
	row, err := mcpDAO.GetServer(uid, id)
	if err != nil {
		return DTO{}, mcpDAO.ErrNotFound
	}
	headers, e := open(row.EncryptedHeaders)
	if e != nil {
		return DTO{}, e
	}
	env, e := open(row.EncryptedEnv)
	if e != nil {
		return DTO{}, e
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	client, e := mcpclient.NewMCPClientWithConfig(ctx, mcpclient.Config{Transport: row.Transport, URL: row.URL, Headers: headers, Command: row.Command, Args: decodeArgs(row.ArgsJSON), Env: env})
	if e != nil {
		return DTO{}, fmt.Errorf("MCP connection failed: %w", e)
	}
	defer client.Close()
	discovered, e := client.ListTools(ctx)
	if e != nil {
		return DTO{}, fmt.Errorf("MCP tool discovery failed: %w", e)
	}
	tools := make([]model.MCPTool, 0, len(discovered))
	for _, t := range discovered {
		raw, _ := json.Marshal(t.InputSchema)
		tools = append(tools, model.MCPTool{ServerID: row.ID, Name: t.Name, Description: t.Description, InputSchemaJSON: string(raw), Enabled: true})
	}
	if e = mcpDAO.ReplaceTools(row.ID, tools); e != nil {
		return DTO{}, e
	}
	now := time.Now()
	row.Status = "ready"
	row.LastTestedAt = &now
	if e = mcpDAO.UpdateServer(row); e != nil {
		return DTO{}, e
	}
	return Get(username, id)
}
