package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOfficialClientDiscoversAndInvokesEinoTool(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	sdk.AddTool(server, &sdk.Tool{Name: "echo", Description: "Echo a message"}, func(_ context.Context, _ *sdk.CallToolRequest, input struct {
		Message string `json:"message" jsonschema:"message to echo"`
	}) (*sdk.CallToolResult, map[string]string, error) {
		return nil, map[string]string{"message": input.Message}, nil
	})
	httpServer := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil))
	defer httpServer.Close()

	ctx := context.Background()
	client, err := NewMCPClient(ctx, httpServer.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	discovered, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(discovered) != 1 || discovered[0].Name != "echo" {
		t.Fatalf("unexpected tools: %#v", discovered)
	}

	adapted, err := NewEinoTool(client, discovered[0])
	if err != nil {
		t.Fatalf("adapt tool: %v", err)
	}
	result, err := adapted.InvokableRun(ctx, `{"message":"hello"}`)
	if err != nil {
		t.Fatalf("invoke tool: %v", err)
	}
	if !strings.Contains(result, `"message":"hello"`) {
		t.Fatalf("unexpected tool result: %s", result)
	}
}

func TestEinoToolPreservesMCPToolError(t *testing.T) {
	server := sdk.NewServer(&sdk.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	sdk.AddTool(server, &sdk.Tool{Name: "fail"}, func(context.Context, *sdk.CallToolRequest, map[string]any) (*sdk.CallToolResult, string, error) {
		return nil, "", context.Canceled
	})
	httpServer := httptest.NewServer(sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil))
	defer httpServer.Close()

	ctx := context.Background()
	client, err := NewMCPClient(ctx, httpServer.URL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()
	tools, err := client.EinoTools(ctx)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	result, err := tools[0].(tool.InvokableTool).InvokableRun(ctx, `{}`)
	if err != nil {
		t.Fatalf("invoke should preserve tool error as result: %v", err)
	}
	if !strings.Contains(result, `"isError":true`) {
		t.Fatalf("expected isError result: %s", result)
	}
}

func TestSchemaFromMCPPreservesRequiredDescription(t *testing.T) {
	params, err := schemaFromMCP(map[string]any{
		"type":     "object",
		"required": []string{"city"},
		"properties": map[string]any{
			"city": map[string]any{"type": "string", "description": "city name"},
		},
	})
	if err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	jsonSchema, err := params.ToJSONSchema()
	if err != nil {
		t.Fatalf("convert schema: %v", err)
	}
	if jsonSchema == nil || jsonSchema.Properties == nil || jsonSchema.Properties.Len() != 1 {
		t.Fatalf("properties were not preserved: %#v", jsonSchema)
	}
}
