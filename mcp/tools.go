// tools.go: tool menu + name→handler dispatch. Schemas live here; work lives in handlers.go.
package mcp

import (
	"context"
	"fmt"
	"sync"
)

// ToolInputSchema describes arguments a tool accepts (JSON Schema-ish).
// Field names match pipeline.ToolInputSchema for easy conversion later.
type ToolInputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

// Tool is one entry for tools/list.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema ToolInputSchema `json:"inputSchema"`
}

// HandlerFunc runs a tool. args come from CallToolParams.Arguments.
type HandlerFunc func(args map[string]interface{}) (string, error)

const requestContextArg = "_mcp_request_context"

// Registry maps tool names to schemas and handlers.
type Registry struct {
	mu       sync.RWMutex
	tools    []Tool
	handlers map[string]HandlerFunc
	approve  func(context.Context, Tool, map[string]interface{}) error
	readOnly map[string]bool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]HandlerFunc), readOnly: make(map[string]bool)}
}

// DefaultRegistry returns the built-in Ubuntu-dev tools.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	registerBuiltin(r)
	return r
}

// Register adds a tool schema and handler. New or replaced tools require approval by default.
func (r *Registry) Register(tool Tool, fn HandlerFunc) { r.register(tool, fn, false) }

// RegisterReadOnly adds an inspection tool which does not intentionally change system state.
func (r *Registry) RegisterReadOnly(tool Tool, fn HandlerFunc) { r.register(tool, fn, true) }

func (r *Registry) register(tool Tool, fn HandlerFunc, readOnly bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[tool.Name] = fn
	r.readOnly[tool.Name] = readOnly
	for i := range r.tools {
		if r.tools[i].Name == tool.Name {
			r.tools[i] = tool
			return
		}
	}
	r.tools = append(r.tools, tool)
}

// SetApprovalHandler installs a human approval boundary for tools not classified as read-only.
// Without a handler, those tools fail closed in clients without an approval UI.
func (r *Registry) SetApprovalHandler(fn func(context.Context, Tool, map[string]interface{}) error) {
	r.mu.Lock()
	r.approve = fn
	r.mu.Unlock()
}

// ListTools returns a copy of registered tool schemas.
func (r *Registry) ListTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, len(r.tools))
	copy(out, r.tools)
	return out
}

// Dispatch runs the named tool. Unknown tools return an error string with isError=true.
func (r *Registry) Dispatch(name string, args map[string]interface{}) (text string, isError bool) {
	return r.DispatchContext(context.Background(), name, args)
}

// DispatchContext runs a tool with the caller context available to handlers that
// support cancellation. The context is injected internally and is not a tool argument.
func (r *Registry) DispatchContext(ctx context.Context, name string, args map[string]interface{}) (text string, isError bool) {
	if name == "" {
		return "empty tool name", true
	}
	r.mu.RLock()
	fn, ok := r.handlers[name]
	var tool Tool
	for _, registered := range r.tools {
		if registered.Name == name {
			tool = registered
			break
		}
	}
	approve := r.approve
	readOnly := r.readOnly[name]
	r.mu.RUnlock()
	if !ok {
		return fmt.Sprintf("unknown tool: %s", name), true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "tool cancelled before execution", true
	}
	toolArgs := make(map[string]interface{}, len(args)+1)
	for key, value := range args {
		toolArgs[key] = value
	}
	if !readOnly {
		if approve == nil {
			return fmt.Sprintf("tool %q requires explicit user approval; this MCP client has no approval handler", name), true
		}
		if err := approve(ctx, tool, toolArgs); err != nil {
			return err.Error(), true
		}
		if err := ctx.Err(); err != nil {
			return "tool cancelled before execution", true
		}
	}
	toolArgs[requestContextArg] = ctx
	text, err := fn(toolArgs)
	if err != nil {
		return err.Error(), true
	}
	return text, false
}
