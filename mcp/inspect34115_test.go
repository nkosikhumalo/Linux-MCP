package mcp

import (
	"strings"
	"testing"
)

func TestInspect34115(t *testing.T) {
	r := DefaultRegistry()
	text, isErr := r.Dispatch("inspect_port", map[string]interface{}{"port": 34115})
	if isErr {
		t.Fatal(text)
	}
	t.Log(text)
	if !strings.Contains(text, "34115") {
		t.Fatalf("expected port in output: %s", text)
	}
}
