package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultRegistryListAndDispatch(t *testing.T) {
	r := DefaultRegistry()
	tools := r.ListTools()
	if len(tools) < 4 {
		t.Fatalf("expected builtin tools, got %d", len(tools))
	}

	text, isErr := r.Dispatch("git_status", map[string]interface{}{"dir": "."})
	if isErr {
		t.Fatal(text)
	}
	if !strings.Contains(text, "exit=") {
		t.Fatal(text)
	}
}

func TestDispatchUnknown(t *testing.T) {
	r := DefaultRegistry()
	text, isErr := r.Dispatch("nope", nil)
	if !isErr || !strings.Contains(text, "unknown tool") {
		t.Fatalf("got %q isErr=%v", text, isErr)
	}
}

func TestRunCommandFailsClosedWithoutApprovalHandler(t *testing.T) {
	r := DefaultRegistry()
	text, isErr := r.Dispatch("run_command", map[string]interface{}{"command": "echo must-not-run"})
	if !isErr || !strings.Contains(text, "requires explicit user approval") {
		t.Fatalf("run_command should fail closed without an approval UI: %q isErr=%v", text, isErr)
	}
}

func TestRunCommandExecutesOnlyAfterApproval(t *testing.T) {
	r := DefaultRegistry()
	approved := false
	r.SetApprovalHandler(func(_ context.Context, _ Tool, _ map[string]interface{}) error {
		approved = true
		return nil
	})
	text, isErr := r.Dispatch("run_command", map[string]interface{}{"command": "printf approved"})
	if isErr || !approved || !strings.Contains(text, "approved") {
		t.Fatalf("approved command did not return its output: approved=%v isErr=%v result=%q", approved, isErr, text)
	}
}

func TestRunCommandDirectsPasswordCommandsToTerminal(t *testing.T) {
	text, err := handleRunCommand(map[string]interface{}{"command": "sudo apt install example"})
	if err != nil || !strings.Contains(text, "run this command in your terminal") {
		t.Fatalf("expected terminal password guidance, got result=%q err=%v", text, err)
	}
}
