package mcp

import (
	"strings"
	"testing"
)

func TestOSInfoAndTop(t *testing.T) {
	r := DefaultRegistry()
	text, errFlag := r.Dispatch("os_info", nil)
	if errFlag {
		t.Fatal(text)
	}
	if !strings.Contains(text, "Ubuntu") && !strings.Contains(text, "NAME=") {
		t.Fatal(text)
	}
	text, errFlag = r.Dispatch("top_processes", map[string]interface{}{"limit": 5})
	if errFlag {
		t.Fatal(text)
	}
	if !strings.Contains(text, "PID") && !strings.Contains(text, "pid") {
		t.Fatal(text)
	}
	text, errFlag = r.Dispatch("find_process", map[string]interface{}{})
	if errFlag {
		t.Fatal(text)
	}
}
