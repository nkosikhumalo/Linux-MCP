// app.go: Wails bridge — JS calls window.go.main.App.Chat / ListTools.
package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	sys "ubuntu-dev-assistant/mcp"
	"ubuntu-dev-assistant/pipeline"
)

// App holds shared backend state for the desktop UI.
type App struct {
	ctx      context.Context
	router   *pipeline.Router
	tools    *sys.Registry
	err      error // setup error (e.g. missing API key)
	store    *conversationStore
	storeErr error
	chatMu   sync.Mutex
	active   *Conversation
}

// NewApp builds App. Missing OPENROUTER_API_KEY is reported on Chat, not at launch.
func NewApp() *App {
	tools := sys.DefaultRegistry()
	store, storeErr := newConversationStore()
	client, err := pipeline.NewClientFromEnv()
	if err != nil {
		return &App{tools: tools, err: err, store: store, storeErr: storeErr}
	}
	return &App{
		tools:    tools,
		router:   pipeline.NewRouter(client, tools),
		store:    store,
		storeErr: storeErr,
	}
}

// Startup is the Wails lifecycle hook.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// Chat runs one user turn through the orchestrator.
func (a *App) Chat(message string) (*pipeline.RunResult, error) {
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	if a.err != nil {
		return nil, a.err
	}
	if a.router == nil {
		return nil, fmt.Errorf("router not configured")
	}
	if a.storeErr != nil {
		return nil, fmt.Errorf("conversation history unavailable: %w", a.storeErr)
	}
	if a.active == nil {
		var err error
		a.active, err = newConversation()
		if err != nil {
			return nil, err
		}
		a.router.ClearHistory()
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	message = strings.TrimSpace(message)
	result, err := a.router.Run(ctx, message)
	if err != nil {
		return result, err
	}
	now := time.Now()
	a.active.Messages = append(a.active.Messages,
		ConversationMessage{Role: "user", Content: message, At: now},
		ConversationMessage{Role: "assistant", Content: result.Reply, At: now},
	)
	if len(a.active.Messages) == 2 {
		a.active.Title = titleFor(message)
	}
	a.active.UpdatedAt = now
	if err := a.store.save(a.active); err != nil {
		return result, fmt.Errorf("reply was generated but conversation history could not be saved: %w", err)
	}
	return result, nil
}

// ClearChat resets conversation memory in the router.
func (a *App) ClearChat() {
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	a.active = nil
	if a.router != nil {
		a.router.ClearHistory()
	}
}

// ListConversations returns saved chats newest first.
func (a *App) ListConversations() ([]ConversationSummary, error) {
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	if a.storeErr != nil {
		return nil, a.storeErr
	}
	return a.store.list()
}

// NewConversation starts a fresh in-memory chat. It is saved after its first reply.
func (a *App) NewConversation() (ConversationSummary, error) {
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	conv, err := newConversation()
	if err != nil {
		return ConversationSummary{}, err
	}
	a.active = conv
	if a.router != nil {
		a.router.ClearHistory()
	}
	return ConversationSummary{ID: conv.ID, Title: conv.Title, CreatedAt: conv.CreatedAt, UpdatedAt: conv.UpdatedAt}, nil
}

// OpenConversation restores a saved chat for display and future model context.
func (a *App) OpenConversation(id string) (*Conversation, error) {
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	if a.storeErr != nil {
		return nil, a.storeErr
	}
	conv, err := a.store.load(id)
	if err != nil {
		return nil, err
	}
	a.active = conv
	history := make([]pipeline.ChatMessage, 0, len(conv.Messages))
	for _, msg := range conv.Messages {
		history = append(history, pipeline.ChatMessage{Role: msg.Role, Content: msg.Content})
	}
	if a.router != nil {
		a.router.SetHistory(history)
	}
	return conv, nil
}

// GetConversation reads a saved chat without changing the currently active conversation.
func (a *App) GetConversation(id string) (*Conversation, error) {
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	if a.storeErr != nil {
		return nil, a.storeErr
	}
	return a.store.load(id)
}

// DeleteConversation removes a saved local chat and clears it if currently open.
func (a *App) DeleteConversation(id string) error {
	a.chatMu.Lock()
	defer a.chatMu.Unlock()
	if a.storeErr != nil {
		return a.storeErr
	}
	if err := a.store.delete(id); err != nil {
		return err
	}
	if a.active != nil && a.active.ID == id {
		a.active = nil
		if a.router != nil {
			a.router.ClearHistory()
		}
	}
	return nil
}

// ListTools returns registered tool schemas for the UI (optional).
func (a *App) ListTools() []sys.Tool {
	if a.tools == nil {
		return nil
	}
	return a.tools.ListTools()
}
