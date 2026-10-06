package mcp

import (
	"testing"

	"github.com/dinghen/CogniGo/model"
)

func TestValidateTransportAndSSRF(t *testing.T) {
	base := Input{Name: "demo", Transport: "streamable-http", URL: "https://example.com/mcp"}
	if err := validate(base); err != nil {
		t.Fatalf("valid HTTP config rejected: %v", err)
	}
	base.URL = "http://127.0.0.1:8080/mcp"
	if err := validate(base); err != ErrPrivateHTTP {
		t.Fatalf("expected private target rejection, got %v", err)
	}
	t.Setenv("COGNIGO_MCP_ALLOW_STDIO", "true")
	t.Setenv("COGNIGO_MCP_STDIO_COMMANDS", "node")
	base = Input{Name: "stdio", Transport: "stdio", Command: "node", Args: []string{"server.js"}}
	if err := validate(base); err != nil {
		t.Fatalf("valid stdio config rejected: %v", err)
	}
	base.Command = "sh -c echo unsafe"
	if err := validate(base); err == nil {
		t.Fatal("shell command was accepted")
	}
}

func TestDTOMasksSecretsAndDecodesCatalog(t *testing.T) {
	row := &model.MCPServer{ID: 7, Name: "server", Transport: "stdio", ArgsJSON: `["server.js"]`, Enabled: true}
	dto := toDTO(row, []model.MCPTool{{ID: 1, ServerID: 7, Name: "echo", InputSchemaJSON: `{"type":"object"}`, Enabled: true}}, map[string]string{"Authorization": "secret"}, map[string]string{"TOKEN": "value"})
	if dto.HeadersMasked["Authorization"] != "********" || dto.EnvMasked["TOKEN"] != "********" {
		t.Fatalf("secrets were not masked: %#v %#v", dto.HeadersMasked, dto.EnvMasked)
	}
	if len(dto.Args) != 1 || dto.Args[0] != "server.js" || string(dto.Tools[0].InputSchema) != `{"type":"object"}` {
		t.Fatalf("catalog was not projected: %#v", dto)
	}
}
