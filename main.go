// Package main
//
// main.go — WHAT BELONGS HERE
// ---------------------------
// App entrypoint only. Keep it short.
//
// TODAY (learning MCP): you start an MCP stdio server here.
//   ServeStdio blocks and waits for a client — it will NOT print
//   "Hello World" by itself. Hello only happens when a client
//   calls the hello_world tool.
//
// LATER (Wails desktop app): this file should instead:
//   - embed frontend/ with //go:embed
//   - create App from app.go
//   - call wails.Run(...) with window options from wails.json
//
// YOU SHOULD NOT PUT HERE:
//   - Lots of tool handlers → move to mcp/
//   - OpenRouter client     → pipeline/
//   - Big UI logic          → frontend/ + app.go bridge
//
package main

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Create a new MCP server (stdio transport — waits for JSON-RPC on stdin)
	s := server.NewMCPServer(
		"Demo 🚀",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	// Register one demo tool. Real Ubuntu tools will live under mcp/ later.
	tool := mcp.NewTool("hello_world",
		mcp.WithDescription("Say hello to someone"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the person to greet"),
		),
	)

	s.AddTool(tool, helloHandler)

	// Blocks forever until the MCP client disconnects or you Ctrl+C
	if err := server.ServeStdio(s); err != nil {
		fmt.Printf("Server error: %v\n", err)
	}
}

// helloHandler runs ONLY when a client calls the hello_world tool.
func helloHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, err := request.RequireString("name")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
}
