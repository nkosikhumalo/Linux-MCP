package mcp

import (
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
