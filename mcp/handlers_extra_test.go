package mcp

import (
	"strings"
	"testing"
)

func TestInspectPortAndDisk(t *testing.T) {
	r := DefaultRegistry()
	text, errFlag := r.Dispatch("disk_free", nil)
	if errFlag {
		t.Fatal(text)
	}
	if !strings.Contains(text, "Filesystem") && !strings.Contains(strings.ToLower(text), "size") && !strings.Contains(text, "/") {
		t.Fatalf("unexpected df output: %s", text)
	}
	text, errFlag = r.Dispatch("inspect_port", map[string]interface{}{"port": 22})
	if errFlag {
		t.Fatal(text)
	}
	if !strings.Contains(text, "ss") {
		t.Fatalf("%s", text)
	}
}
