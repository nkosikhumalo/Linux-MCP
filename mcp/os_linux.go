// Package mcp — STEP 1: Linux System Tool Engine.
// os_linux.go: safe argv exec (no shell). Handlers call this; they do not exec themselves.
package mcp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	maxOutputBytes = 256 * 1024 // cap so a runaway command cannot fill memory
)

// ExecResult is the captured outcome of one local process.
type ExecResult struct {
	Command  string
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// RunCommand runs name with args under ctx (default 30s). No shell.
func RunCommand(ctx context.Context, name string, args ...string) (*ExecResult, error) {
	return run(ctx, "", name, args...)
}

// RunInDir is RunCommand with an explicit working directory.
func RunInDir(ctx context.Context, dir, name string, args ...string) (*ExecResult, error) {
	return run(ctx, dir, name, args...)
}

// StartDetached launches a process and returns immediately (for GUI apps).
// Inherits DISPLAY/WAYLAND from the current environment.
func StartDetached(name string, args ...string) (int, error) {
	if err := validateArgv(name, args); err != nil {
		return 0, err
	}
	return startDetached(name, args...)
}

// StartDetachedTrusted launches argv from a trusted .desktop Exec= line.
// Absolute paths are allowed (Chrome PWAs, flatpak helpers).
func StartDetachedTrusted(argv []string) (int, error) {
	if len(argv) == 0 {
		return 0, fmt.Errorf("empty argv")
	}
	for i, a := range argv {
		if strings.IndexByte(a, 0) >= 0 {
			return 0, fmt.Errorf("argument %d contains NUL", i)
		}
	}
	return startDetached(argv[0], argv[1:]...)
}

func startDetached(name string, args ...string) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }()
	return pid, nil
}

func run(ctx context.Context, dir, name string, args ...string) (*ExecResult, error) {
	if err := validateArgv(name, args); err != nil {
		return nil, err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{buf: &stdout, max: maxOutputBytes}
	cmd.Stderr = &limitedWriter{buf: &stderr, max: maxOutputBytes}

	err := cmd.Run()
	res := &ExecResult{
		Command:  strings.TrimSpace(name + " " + strings.Join(args, " ")),
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		Duration: time.Since(start).Round(time.Millisecond),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
			return res, nil // non-zero exit is a result, not a transport failure
		}
		if ctx.Err() != nil {
			return res, fmt.Errorf("command cancelled or timed out: %w", ctx.Err())
		}
		return res, fmt.Errorf("exec %s: %w", name, err)
	}
	return res, nil
}

// FormatExec renders an ExecResult as text for tool / AI responses.
func FormatExec(r *ExecResult) string {
	if r == nil {
		return "(no result)"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "$ %s\n", r.Command)
	if r.Stdout != "" {
		fmt.Fprintf(&b, "--- stdout ---\n%s\n", r.Stdout)
	}
	if r.Stderr != "" {
		fmt.Fprintf(&b, "--- stderr ---\n%s\n", r.Stderr)
	}
	fmt.Fprintf(&b, "exit=%d duration=%s\n", r.ExitCode, r.Duration)
	return b.String()
}

func validateArgv(name string, args []string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("empty command name")
	}
	// Basename only — blocks absolute/relative paths like /bin/sh or ../evil.
	if strings.ContainsAny(name, `/\:`) {
		return fmt.Errorf("command name must be a bare binary, got %q", name)
	}
	for i, a := range args {
		if strings.IndexByte(a, 0) >= 0 {
			return fmt.Errorf("argument %d contains NUL", i)
		}
	}
	return nil
}

// limitedWriter stops accepting data after max bytes (appends a marker once).
type limitedWriter struct {
	buf *bytes.Buffer
	max int
	hit bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remain := w.max - w.buf.Len()
	if remain <= 0 {
		w.hit = true
		return len(p), nil
	}
	if len(p) > remain {
		_, _ = w.buf.Write(p[:remain])
		if !w.hit {
			_, _ = w.buf.WriteString("\n...[truncated]...\n")
			w.hit = true
		}
		return len(p), nil
	}
	return w.buf.Write(p)
}
