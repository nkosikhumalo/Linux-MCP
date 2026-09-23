// app.go: Wails bridge — JS calls window.go.main.App.Chat / ListTools.
package main

import (
	"context"
	"fmt"

	sys "ubuntu-dev-assistant/mcp"
	"ubuntu-dev-assistant/pipeline"
)

// App holds shared backend state for the desktop UI.
type App struct {
	ctx    context.Context
	router *pipeline.Router
	tools  *sys.Registry
	err    error // setup error (e.g. missing API key)
}

// NewApp builds App. Missing OPENROUTER_API_KEY is reported on Chat, not at launch.
func NewApp() *App {
	tools := sys.DefaultRegistry()
	client, err := pipeline.NewClientFromEnv()
	if err != nil {
		return &App{tools: tools, err: err}
	}
	return &App{
		tools:  tools,
		router: pipeline.NewRouter(client, tools),
	}
}

// Startup is the Wails lifecycle hook.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// Chat runs one user turn through the orchestrator.
func (a *App) Chat(message string) (*pipeline.RunResult, error) {
	if a.err != nil {
		return nil, a.err
	}
	if a.router == nil {
		return nil, fmt.Errorf("router not configured")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.router.Run(ctx, message)
}

// ClearChat resets conversation memory in the router.
func (a *App) ClearChat() {
	if a.router != nil {
		a.router.ClearHistory()
	}
}

// ListTools returns registered tool schemas for the UI (optional).
func (a *App) ListTools() []sys.Tool {
	if a.tools == nil {
		return nil
	}
	return a.tools.ListTools()
}
