// tools.go: tool menu + name→handler dispatch. Schemas live here; work lives in handlers.go.
package mcp

import (
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

// Registry maps tool names to schemas and handlers.
type Registry struct {
	mu       sync.RWMutex
	tools    []Tool
	handlers map[string]HandlerFunc
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]HandlerFunc)}
}

// DefaultRegistry returns the built-in Ubuntu-dev tools.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	registerBuiltin(r)
	return r
}

// Register adds a tool schema and its handler. Re-registering the same name replaces the handler.
func (r *Registry) Register(tool Tool, fn HandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[tool.Name] = fn
	for i := range r.tools {
		if r.tools[i].Name == tool.Name {
			r.tools[i] = tool
			return
		}
	}
	r.tools = append(r.tools, tool)
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
	if name == "" {
		return "empty tool name", true
	}
	r.mu.RLock()
	fn, ok := r.handlers[name]
	r.mu.RUnlock()
	if !ok {
		return fmt.Sprintf("unknown tool: %s", name), true
	}
	if args == nil {
		args = map[string]interface{}{}
	}
	text, err := fn(args)
	if err != nil {
		return err.Error(), true
	}
	return text, false
}
