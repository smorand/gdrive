package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"gdrive/internal/drive"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	driveapi "google.golang.org/api/drive/v3"
	"google.golang.org/api/driveactivity/v2"
	"google.golang.org/api/option"
)

// setupToolTest creates a test MCP server backed by a mock Drive API.
// It returns the server and a context for tool invocations.
func setupToolTest(t *testing.T) *Server {
	t.Helper()

	mockServer, _ := newMockDriveServer(t)

	// Override drive service to use mock
	origDrive := driveServiceOverride
	driveServiceOverride = func(ctx context.Context) (*drive.Service, error) {
		svc, err := driveapi.NewService(ctx,
			option.WithEndpoint(mockServer.URL),
			option.WithoutAuthentication(),
		)
		if err != nil {
			return nil, err
		}
		return drive.NewService(svc), nil
	}
	t.Cleanup(func() { driveServiceOverride = origDrive })

	// Override activity service to use mock
	origActivity := activityServiceOverride
	activityServiceOverride = func(ctx context.Context) (*driveactivity.Service, error) {
		svc, err := driveactivity.NewService(ctx,
			option.WithEndpoint(mockServer.URL),
			option.WithoutAuthentication(),
		)
		if err != nil {
			return nil, err
		}
		return svc, nil
	}
	t.Cleanup(func() { activityServiceOverride = origActivity })

	// Create real MCP server with test credentials
	tmpDir := t.TempDir()
	credFile := tmpDir + "/creds.json"
	os.WriteFile(credFile, []byte(`{"client_id":"test-id","client_secret":"test-secret"}`), 0600)

	cfg := &ServerConfig{
		Host:           "127.0.0.1",
		Port:           0,
		BaseURL:        "http://localhost:8080",
		CredentialFile: credFile,
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}

	return srv
}

// callTool invokes an MCP tool via JSON-RPC HandleMessage.
func callTool(t *testing.T, srv *Server, toolName string, args map[string]interface{}) (*mcplib.CallToolResult, error) {
	t.Helper()

	params := map[string]interface{}{
		"name":      toolName,
		"arguments": args,
	}

	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  params,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal tool call: %v", err)
	}

	ctx := context.Background()
	resp := srv.mcpServer.HandleMessage(ctx, msgBytes)

	respBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	var rpcResp struct {
		Result *mcplib.CallToolResult `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &rpcResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v\nraw: %s", err, string(respBytes))
	}

	if rpcResp.Error != nil {
		return nil, &toolError{Code: rpcResp.Error.Code, Message: rpcResp.Error.Message}
	}

	return rpcResp.Result, nil
}

// toolError represents an MCP tool error.
type toolError struct {
	Code    int
	Message string
}

func (e *toolError) Error() string {
	return e.Message
}

// extractResultJSON parses the text content from a CallToolResult into a map.
func extractResultJSON(t *testing.T, result *mcplib.CallToolResult) map[string]interface{} {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}

	for _, content := range result.Content {
		if textContent, ok := content.(mcplib.TextContent); ok {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(textContent.Text), &data); err != nil {
				t.Fatalf("failed to parse result JSON: %v\nraw: %s", err, textContent.Text)
			}
			return data
		}
	}

	t.Fatal("no text content in result")
	return nil
}

// extractResultArray parses the text content from a CallToolResult into a slice.
func extractResultArray(t *testing.T, result *mcplib.CallToolResult) []interface{} {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}

	for _, content := range result.Content {
		if textContent, ok := content.(mcplib.TextContent); ok {
			var data []interface{}
			if err := json.Unmarshal([]byte(textContent.Text), &data); err != nil {
				t.Fatalf("failed to parse result JSON array: %v\nraw: %s", err, textContent.Text)
			}
			return data
		}
	}

	t.Fatal("no text content in result")
	return nil
}
