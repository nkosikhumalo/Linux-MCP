// router.go: pick model, keep chat history, loop model↔tools until a final answer.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	sys "ubuntu-dev-assistant/mcp"
)

const (
	defaultFastModel  = "openai/gpt-4o-mini"
	defaultHeavyModel = "openai/gpt-4o-mini"
	defaultMaxRounds  = 6
	defaultMaxTokens  = 1024
	maxHistoryMsgs    = 16 // user+assistant pairs kept (approx)
)

// Router orchestrates chat + local tool calls.
type Router struct {
	Client     *Client
	Tools      *sys.Registry
	FastModel  string
	HeavyModel string
	MaxRounds  int
	MaxTokens  int

	mu      sync.Mutex
	history []ChatMessage // prior user/assistant turns (no tool spam)
}

// RunResult is what the UI / App bridge needs.
type RunResult struct {
	Reply string
	Trace []Step
}

// NewRouter wires defaults (overridable via .env).
func NewRouter(client *Client, tools *sys.Registry) *Router {
	LoadDotEnvDefault()
	fastModel := envOr("OPENROUTER_FAST_MODEL", defaultFastModel)
	heavyModel := envOr("OPENROUTER_HEAVY_MODEL", defaultHeavyModel)
	if client != nil && client.local {
		model := strings.TrimSpace(os.Getenv("LOCAL_MODEL_NAME"))
		fastModel, heavyModel = model, model
	}
	return &Router{
		Client:     client,
		Tools:      tools,
		FastModel:  fastModel,
		HeavyModel: heavyModel,
		MaxRounds:  defaultMaxRounds,
		MaxTokens:  envInt("OPENROUTER_MAX_TOKENS", defaultMaxTokens),
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n <= 0 {
		return fallback
	}
	return n
}

// ClearHistory resets the conversation (UI can call this later).
func (r *Router) ClearHistory() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.history = nil
	r.mu.Unlock()
}

// SetHistory replaces the in-memory context used for the next chat turn.
func (r *Router) SetHistory(history []ChatMessage) {
	if r == nil {
		return
	}
	clean := make([]ChatMessage, 0, len(history))
	for _, msg := range history {
		if (msg.Role == "user" || msg.Role == "assistant") && strings.TrimSpace(msg.Content) != "" {
			clean = append(clean, ChatMessage{Role: msg.Role, Content: msg.Content})
		}
	}
	if len(clean) > maxHistoryMsgs {
		clean = clean[len(clean)-maxHistoryMsgs:]
	}
	r.mu.Lock()
	r.history = clean
	r.mu.Unlock()
}

// Run sends userMessage through the model/tool loop, with conversation memory.
func (r *Router) Run(ctx context.Context, userMessage string) (*RunResult, error) {
	tr := NewTracer()
	if r == nil || r.Client == nil {
		return &RunResult{Trace: tr.Snapshot()}, fmt.Errorf("router not configured")
	}
	userMessage = strings.TrimSpace(userMessage)
	if userMessage == "" {
		return &RunResult{Trace: tr.Snapshot()}, fmt.Errorf("empty message")
	}

	model := r.pickModel(userMessage)
	tr.Add("route_decision", model, "fast vs heavy heuristic")

	r.mu.Lock()
	prior := append([]ChatMessage(nil), r.history...)
	r.mu.Unlock()

	messages := []ChatMessage{{Role: "system", Content: systemPrompt}}
	messages = append(messages, prior...)
	messages = append(messages, ChatMessage{Role: "user", Content: userMessage})

	tools := openAITools(r.Tools)
	max := r.MaxRounds
	if max <= 0 {
		max = defaultMaxRounds
	}

	forceTextOnly := false
	for round := 0; round < max; round++ {
		tr.Add("model_request", model, fmt.Sprintf("round %d", round+1))
		maxTokens := r.MaxTokens
		if maxTokens <= 0 {
			maxTokens = defaultMaxTokens
		}
		req := CompletionRequest{
			Model:     model,
			Messages:  messages,
			Tools:     tools,
			MaxTokens: maxTokens,
		}
		switch {
		case forceTextOnly:
			// After tools: require a normal chat reply (no more tool spam).
			req.ToolChoice = "none"
		case round == 0 && len(tools) > 0 && wantsSystemFacts(userMessage) && !isPolicyOrMeta(userMessage):
			req.ToolChoice = "required"
		}

		res, err := r.Client.ChatCompletion(ctx, req)
		if err != nil {
			tr.Add("error", "chat_completion", err.Error())
			return &RunResult{Trace: tr.Snapshot()}, err
		}

		msg := res.Choices[0].Message
		tr.Add("model_response", model, summarizeMsg(msg))

		if len(msg.ToolCalls) == 0 {
			reply := strings.TrimSpace(msg.Content)
			r.appendHistory(userMessage, reply)
			return &RunResult{Reply: reply, Trace: tr.Snapshot()}, nil
		}

		messages = append(messages, ChatMessage{
			Role:      "assistant",
			Content:   msg.Content,
			ToolCalls: msg.ToolCalls,
		})

		var results []string
		allDoneActions := len(msg.ToolCalls) > 0
		for _, tc := range msg.ToolCalls {
			name := tc.Function.Name
			args := parseArgs(tc.Function.Arguments)
			tr.Add("tool_call", name, tc.Function.Arguments)

			text := "no tool registry configured"
			if r.Tools != nil {
				text, _ = r.Tools.DispatchContext(ctx, name, args)
			}
			tr.Add("tool_result", name, truncate(text, 500))
			results = append(results, text)

			messages = append(messages, ChatMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       name,
				Content:    text,
			})
			if !isDoneActionTool(name) {
				allDoneActions = false
			}
			if ctx.Err() != nil {
				reply := "The request was stopped before the tool finished. Its output is partial and is not a completed result.\n\n" + truncate(text, 2400)
				tr.Add("route_decision", "cancelled_tool", name)
				r.appendHistory(userMessage, reply)
				return &RunResult{Reply: reply, Trace: tr.Snapshot()}, nil
			}
		}

		// Simple open/close/uri: polish locally (instant). Trace stays backend-only.
		if allDoneActions {
			reply := friendlyActionReply(msg.ToolCalls, results)
			tr.Add("route_decision", "user_reply", "polished action summary")
			r.appendHistory(userMessage, reply)
			return &RunResult{Reply: reply, Trace: tr.Snapshot()}, nil
		}

		forceTextOnly = true
	}

	reply := "I could not finish within the tool limit. I will not bypass permissions or invent results — ask something my tools can do, or rephrase."
	tr.Add("error", "max_rounds", reply)
	r.appendHistory(userMessage, reply)
	return &RunResult{Reply: reply, Trace: tr.Snapshot()}, nil
}

func (r *Router) appendHistory(user, assistant string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history = append(r.history,
		ChatMessage{Role: "user", Content: user},
		ChatMessage{Role: "assistant", Content: assistant},
	)
	// Keep last maxHistoryMsgs messages.
	if len(r.history) > maxHistoryMsgs {
		r.history = r.history[len(r.history)-maxHistoryMsgs:]
	}
}

func (r *Router) pickModel(msg string) string {
	fast, heavy := r.FastModel, r.HeavyModel
	if fast == "" {
		fast = defaultFastModel
	}
	if heavy == "" {
		heavy = defaultHeavyModel
	}
	lower := strings.ToLower(msg)
	heavyHints := []string{
		"debug", "journal", "systemd", "fail", "error", "stack",
		"test", "race", "refactor", "architect", "port", "ss ",
		"git ", "diff", "why", "investigate",
	}
	if utf8.RuneCountInString(msg) > 280 {
		return heavy
	}
	for _, h := range heavyHints {
		if strings.Contains(lower, h) {
			return heavy
		}
	}
	return fast
}

func openAITools(reg *sys.Registry) []ChatTool {
	if reg == nil {
		return nil
	}
	list := reg.ListTools()
	out := make([]ChatTool, 0, len(list))
	for _, t := range list {
		out = append(out, ChatTool{
			Type: "function",
			Function: ChatFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters: ToolInputSchema{
					Type:       t.InputSchema.Type,
					Properties: t.InputSchema.Properties,
					Required:   t.InputSchema.Required,
				},
			},
		})
	}
	return out
}

func parseArgs(raw string) map[string]interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]interface{}{}
	}
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return map[string]interface{}{}
	}
	return args
}

func summarizeMsg(m ChatMessage) string {
	if len(m.ToolCalls) > 0 {
		names := make([]string, 0, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			names = append(names, tc.Function.Name)
		}
		return "tool_calls: " + strings.Join(names, ", ")
	}
	return truncate(m.Content, 200)
}

func wantsSystemFacts(msg string) bool {
	lower := strings.ToLower(msg)
	hints := []string{
		"port", "listen", "process", "disk", "storage", "space",
		"memory", "ram", "slow", "slwo", "cpu", "journal", "log",
		"git", "test", "what is running", "what's running", "taking up",
		"stop ", "kill ", "close ", "quit ",
		"open ", "launch ", "start ", "run ",
		"app", "flatpak", "snap", "pwa",
		"malware", "virus", "clam", "scan", "security",
		"os", "ubuntu", "distro", "operating system",
	}
	for _, h := range hints {
		if strings.Contains(lower, h) {
			return true
		}
	}
	return false
}

func isPolicyOrMeta(msg string) bool {
	lower := strings.ToLower(msg)
	for _, h := range []string{
		"bypass", "sudo", "as root", "elevate", "permission",
		"sensitive", "force access", "ignore permission", "without permission",
	} {
		if strings.Contains(lower, h) {
			return true
		}
	}
	return false
}

func isDoneActionTool(name string) bool {
	switch name {
	case "open_app", "open_uri", "stop_process", "close_app":
		return true
	default:
		return false
	}
}

func friendlyActionReply(calls []ToolCall, results []string) string {
	var parts []string
	for i, tc := range calls {
		res := ""
		if i < len(results) {
			res = results[i]
		}
		args := parseArgs(tc.Function.Arguments)
		name := strings.TrimSpace(strMap(args, "name"))
		if name == "" {
			name = strings.TrimSpace(strMap(args, "uri"))
		}

		switch {
		case strings.Contains(res, "status=fail"):
			reason := extractField(res, "reason")
			if reason == "" {
				reason = "it did not work"
			}
			parts = append(parts, fmt.Sprintf("Could not do that for **%s** — %s.", displayName(name), reason))
		case tc.Function.Name == "open_app" && strings.Contains(res, "status=ok"):
			kind := extractField(res, "kind")
			label := extractField(res, "name")
			if label == "" {
				label = name
			}
			label = strings.Trim(label, `"`)
			note := extractField(res, "note")
			msg := fmt.Sprintf("Opened **%s**", label)
			if kind != "" {
				msg += fmt.Sprintf(" (%s)", kind)
			}
			msg += "."
			if note != "" {
				msg += " " + strings.Trim(note, `"`)
			}
			parts = append(parts, msg)
		case tc.Function.Name == "open_uri" && strings.Contains(res, "status=ok"):
			uri := extractField(res, "uri")
			if uri == "" {
				uri = name
			}
			parts = append(parts, fmt.Sprintf("Opened %s.", uri))
		case (tc.Function.Name == "close_app" || tc.Function.Name == "stop_process") && strings.Contains(res, "status=ok"):
			label := extractField(res, "name")
			if label == "" {
				label = name
			}
			label = strings.Trim(label, `"`)
			parts = append(parts, fmt.Sprintf("Closed **%s**.", label))
		case tc.Function.Name == "stop_process" && strings.Contains(res, "sent SIGTERM"):
			parts = append(parts, fmt.Sprintf("Closed **%s**.", displayName(name)))
		case tc.Function.Name == "stop_process":
			parts = append(parts, fmt.Sprintf("Could not find **%s** running.", displayName(name)))
		default:
			if strings.Contains(res, "status=ok") {
				parts = append(parts, "Done.")
			} else {
				parts = append(parts, "That did not work.")
			}
		}
	}
	if len(parts) == 0 {
		return "Done."
	}
	return strings.Join(parts, " ")
}

func displayName(name string) string {
	if name == "" {
		return "it"
	}
	return name
}

func strMap(args map[string]interface{}, key string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func extractField(res, key string) string {
	prefix := key + "="
	idx := strings.Index(res, prefix)
	if idx < 0 {
		return ""
	}
	rest := res[idx+len(prefix):]
	if strings.HasPrefix(rest, "\"") {
		rest = rest[1:]
		end := strings.Index(rest, "\"")
		if end >= 0 {
			return rest[:end]
		}
		return rest
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return strings.Trim(fields[0], `"`)
}

const systemPrompt = `You are Ubuntu Dev Assistant on the user's local Linux machine. Tools run as the current user.

How to talk:
- Write like a normal chat. Use Markdown when useful: **bold**, headings, lists.
- Never dump raw tool transcripts. Summarize facts from tools.
- NEVER tell the user to run terminal commands yourself when a tool exists (os_info, top_processes, security_scan, mem_free, disk_*, open_app, etc.).
- If a tool returns status=fail, say it failed — never fake success.

Diagnostics:
- Slow PC / high CPU: use top_processes + mem_free (+ disk_free if useful). Name the heaviest processes from the tool output.
- Storage cleanup questions: use disk_free and storage_audit (and disk_usage for folder totals). Report large file paths and sizes, let the user inspect them with open_uri, and never delete or imply an app/file is unused without evidence. Explain safe options and get explicit user direction before any cleanup action. For installed apps, use list_apps to check names; alternatives are suggestions only and should fit the app purpose. Use app_install_history for dated APT/DEB package events; explain that logs may include dependencies or be incomplete. Do not infer dates for Snap, Flatpak, PWA, or manual installs when their date is unavailable.
- "What OS am I on?": use os_info — never ask them to run lsb_release.
- Malware / virus / "do a scan": use security_scan. If ClamAV is missing, say so. Do NOT search for a process literally named "malware". ClamAV detections stream into the desktop chat while it scans; scans wait for completion by default, and the user can press Stop. Treat stopped or timed-out scans as incomplete.
- You cannot sudo-install packages. Say that clearly if they ask you to install antivirus; you CAN scan if clamscan is already installed.
- "Run the tests" with a performance question means system diagnostics (top_processes), not go test, unless they mention Go/code.

Apps:
- list_apps / open_app / close_app for apt/snap/flatpak/PWA/web. Websites are browser tabs — close_app will not kill Chrome to close WhatsApp Web.

Stay on topic. Be concise and honest.`
