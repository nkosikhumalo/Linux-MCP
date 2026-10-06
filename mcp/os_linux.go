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
	Command   string
	Stdout    string
	Stderr    string
	ExitCode  int
	Duration  time.Duration
	TimedOut  bool
	Cancelled bool
}

// RunCommand runs name with args under ctx (default 30s). No shell.
func RunCommand(ctx context.Context, name string, args ...string) (*ExecResult, error) {
	return run(ctx, "", nil, name, args...)
}

// RunCommandObserved runs a command and passes each stdout chunk to observe as it arrives.
// The callback runs on the stdout copy goroutine and should return quickly.
func RunCommandObserved(ctx context.Context, name string, observe func([]byte), args ...string) (*ExecResult, error) {
	return run(ctx, "", observe, name, args...)
}

// RunInDir is RunCommand with an explicit working directory.
func RunInDir(ctx context.Context, dir, name string, args ...string) (*ExecResult, error) {
	return run(ctx, dir, nil, name, args...)
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

func run(ctx context.Context, dir string, observe func([]byte), name string, args ...string) (*ExecResult, error) {
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

	stdout := &limitedWriter{buf: &bytes.Buffer{}, max: maxOutputBytes, observe: observe}
	stderr := &limitedWriter{buf: &bytes.Buffer{}, max: maxOutputBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	res := &ExecResult{
		Command:  strings.TrimSpace(name + " " + strings.Join(args, " ")),
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		Duration: time.Since(start).Round(time.Millisecond),
	}

	if err != nil {
		if ctx.Err() != nil {
			res.TimedOut = ctx.Err() == context.DeadlineExceeded
			res.Cancelled = ctx.Err() == context.Canceled
			res.ExitCode = -1
			return res, nil // preserve partial output so callers can report an incomplete scan
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
			return res, nil // non-zero exit is a result, not a transport failure
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
	if r.TimedOut {
		b.WriteString("process timed out; output above may be partial\n")
	} else if r.Cancelled {
		b.WriteString("process was cancelled; output above may be partial\n")
	}
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

// limitedWriter bounds captured output while preserving both the beginning and end,
// where tools such as clamscan often print their final summary.
type limitedWriter struct {
	buf     *bytes.Buffer
	max     int
	hit     bool
	tail    []byte
	observe func([]byte)
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	inputLen := len(p)
	if w.observe != nil {
		w.observe(p)
	}
	limit := w.max / 2
	if limit < 1 {
		limit = w.max
	}
	remain := limit - w.buf.Len()
	if remain > 0 {
		keep := len(p)
		if keep > remain {
			keep = remain
		}
		_, _ = w.buf.Write(p[:keep])
		p = p[keep:]
	}
	if len(p) > 0 {
		w.hit = true
		keepTail := w.max - limit
		w.tail = append(w.tail, p...)
		if len(w.tail) > keepTail {
			w.tail = append([]byte(nil), w.tail[len(w.tail)-keepTail:]...)
		}
	}
	return inputLen, nil
}

func (w *limitedWriter) String() string {
	if !w.hit {
		return w.buf.String()
	}
	return w.buf.String() + "\n...[output truncated; showing beginning and end]...\n" + string(w.tail)
}
