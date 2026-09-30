package mcp

import (
	"strings"
	"testing"
)

func TestRunCommandUname(t *testing.T) {
	r, err := RunCommand(nil, "uname", "-s")
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 0 || r.Stdout == "" {
		t.Fatalf("unexpected: %+v", r)
	}
	out := FormatExec(r)
	if !strings.Contains(out, "exit=0") {
		t.Fatal(out)
	}
}

func TestRejectPath(t *testing.T) {
	_, err := RunCommand(nil, "/bin/sh", "-c", "echo hi")
	if err == nil {
		t.Fatal("expected path rejection")
	}
}

func TestRejectNUL(t *testing.T) {
	_, err := RunCommand(nil, "echo", "a\x00b")
	if err == nil {
		t.Fatal("expected NUL rejection")
	}
}
