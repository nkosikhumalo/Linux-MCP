package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	sys "ubuntu-dev-assistant/mcp"
)

// MCP stdio server (separate from the Wails desktop UI).
// Run: go run ./cmd/mcp-server
func main() {
	reg := sys.DefaultRegistry()
	// A stdio MCP server has no approval dialog. Keep mutations fail-closed by
	// default, while allowing an operator to explicitly enable app removal.
	if os.Getenv("MCP_ALLOW_APP_REMOVAL") == "true" {
		reg.SetApprovalHandler(func(ctx context.Context, tool sys.Tool, _ map[string]interface{}) error {
			if tool.Name != "remove_app" {
				return fmt.Errorf("tool %q requires an approval UI; MCP_ALLOW_APP_REMOVAL only enables remove_app", tool.Name)
			}
			return nil
		})
	}
	s := server.NewMCPServer(
		"Ubuntu Dev Assistant",
		"0.1.0",
		server.WithToolCapabilities(true),
	)
	for _, t := range reg.ListTools() {
		s.AddTool(toMCPTool(t), dispatch(reg, t.Name))
	}
	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

func toMCPTool(t sys.Tool) mcp.Tool {
	raw, _ := json.Marshal(t.InputSchema)
	return mcp.NewToolWithRawSchema(t.Name, t.Description, raw)
}

func dispatch(reg *sys.Registry, name string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, isErr := reg.DispatchContext(ctx, name, req.GetArguments())
		if isErr {
			return mcp.NewToolResultError(text), nil
		}
		return mcp.NewToolResultText(text), nil
	}
}
