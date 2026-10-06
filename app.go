// app.go: Wails bridge — JS calls window.go.main.App.Chat / ListTools.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	sys "ubuntu-dev-assistant/mcp"
	"ubuntu-dev-assistant/pipeline"
)

// App holds shared backend state for the desktop UI.
type App struct {
	ctx              context.Context
	router           *pipeline.Router
	tools            *sys.Registry
	err              error // setup error (e.g. missing API key)
	store            *conversationStore
	storeErr         error
	chatMu           sync.Mutex
	cancelMu         sync.Mutex
	chatCancel       context.CancelFunc
	active           *Conversation
	approvalMu       sync.Mutex
	pendingApprovals map[string]chan bool
	approvalSeq      uint64
}

// NewApp builds App. Model configuration errors are reported on Chat, not at launch.
func NewApp() *App {
	tools := sys.DefaultRegistry()
	store, storeErr := newConversationStore()
	client, err := pipeline.NewClientFromEnv()
	a := &App{tools: tools, store: store, storeErr: storeErr, pendingApprovals: make(map[string]chan bool)}
	tools.SetApprovalHandler(a.requestToolApproval)
	if err != nil {
		a.err = err
		return a
	}
	a.router = pipeline.NewRouter(client, tools)
	return a
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
	runCtx, cancel := context.WithCancel(ctx)
	runCtx = sys.WithSecurityScanObserver(runCtx, func(event sys.SecurityScanEvent) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "security-scan-update", event)
		}
	})
	a.cancelMu.Lock()
	a.chatCancel = cancel
	a.cancelMu.Unlock()
	defer func() {
		cancel()
		a.cancelMu.Lock()
		a.chatCancel = nil
		a.cancelMu.Unlock()
	}()
	result, err := a.router.Run(runCtx, message)
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

// requestToolApproval pauses a sensitive tool call until the desktop user approves it.
// The standalone MCP server has no approval handler and therefore fails closed.
func (a *App) requestToolApproval(ctx context.Context, tool sys.Tool, args map[string]interface{}) error {
	if a.ctx == nil {
		return fmt.Errorf("tool %q denied: desktop approval UI is unavailable", tool.Name)
	}
	a.approvalMu.Lock()
	a.approvalSeq++
	id := fmt.Sprintf("approval-%d-%d", time.Now().UnixNano(), a.approvalSeq)
	answer := make(chan bool, 1)
	a.pendingApprovals[id] = answer
	a.approvalMu.Unlock()
	defer func() { a.approvalMu.Lock(); delete(a.pendingApprovals, id); a.approvalMu.Unlock() }()
	runtime.EventsEmit(a.ctx, "tool-approval-request", map[string]interface{}{"id": id, "name": tool.Name, "description": tool.Description, "arguments": args})
	select {
	case approved := <-answer:
		if !approved {
			return fmt.Errorf("user denied tool %q", tool.Name)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("tool %q approval cancelled", tool.Name)
	}
}

// ApproveToolCall resolves one pending desktop approval request.
func (a *App) ApproveToolCall(id string, approved bool) bool {
	a.approvalMu.Lock()
	answer := a.pendingApprovals[id]
	a.approvalMu.Unlock()
	if answer == nil {
		return false
	}
	select {
	case answer <- approved:
		return true
	default:
		return false
	}
}

// CancelChat cancels the active model/tool request, including a running ClamAV scan.
func (a *App) CancelChat() bool {
	a.cancelMu.Lock()
	cancel := a.chatCancel
	a.cancelMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
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

// ExportConversation opens a native save dialog and writes one saved chat as JSON.
func (a *App) ExportConversation(id string) (string, error) {
	conv, err := a.GetConversation(id)
	if err != nil {
		return "", err
	}
	ctx := a.ctx
	if ctx == nil {
		return "", fmt.Errorf("save dialog is only available in the desktop app")
	}
	filename := safeConversationFilename(conv.Title) + ".json"
	path, err := runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title:                "Export conversation as JSON",
		DefaultFilename:      filename,
		CanCreateDirectories: true,
		Filters:              []runtime.FileFilter{{DisplayName: "JSON files (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return path, err
	}
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return "", fmt.Errorf("choose a filename ending in .json; only JSON export is supported")
	}
	data, err := json.MarshalIndent(conv, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("export conversation: %w", err)
	}
	return path, nil
}

func safeConversationFilename(title string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(title) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			out.WriteRune(r)
		} else if out.Len() > 0 && !strings.HasSuffix(out.String(), "-") {
			out.WriteByte('-')
		}
	}
	name := strings.Trim(out.String(), "-")
	if name == "" {
		return "conversation"
	}
	return name
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
