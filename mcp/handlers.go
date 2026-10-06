// handlers.go: one function per tool. Parse args → RunCommand/RunInDir → FormatExec.
package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func registerBuiltin(r *Registry) {
	r.RegisterReadOnly(Tool{
		Name:        "inspect_port",
		Description: "Show which process owns a TCP/UDP port using ss -ltnp/-lunp. Use this for questions like 'what is on port 34115'.",
		InputSchema: objectSchema(map[string]interface{}{
			"port": map[string]interface{}{
				"type": "integer", "description": "Port number (1-65535).",
			},
			"protocol": map[string]interface{}{
				"type": "string", "enum": []string{"tcp", "udp", "all"},
			},
		}, "port"),
	}, handleInspectPort)

	r.RegisterReadOnly(Tool{
		Name:        "list_listening_ports",
		Description: "List listening sockets via ss (with process names when available).",
		InputSchema: ToolInputSchema{
			Type: "object",
			Properties: map[string]interface{}{
				"protocol": map[string]interface{}{
					"type": "string", "enum": []string{"tcp", "udp", "all"},
				},
			},
		},
	}, handleListListeningPorts)

	r.RegisterReadOnly(Tool{
		Name:        "disk_free",
		Description: "Show filesystem free/used space via df -h. Use for remaining disk size questions.",
		InputSchema: objectSchema(map[string]interface{}{}),
	}, handleDiskFree)

	r.RegisterReadOnly(Tool{
		Name:        "disk_usage",
		Description: "Show directory sizes via du -h (one level). Cannot read dirs you lack permission for — it will not bypass. Use for storage questions.",
		InputSchema: objectSchema(map[string]interface{}{
			"path": map[string]interface{}{
				"type": "string", "description": "Directory to measure (default /home).",
			},
			"depth": map[string]interface{}{
				"type": "integer", "description": "Max depth 1-2 (default 1).",
			},
		}),
	}, handleDiskUsage)

	r.RegisterReadOnly(Tool{
		Name:        "storage_audit",
		Description: "Read-only scan for the largest files under a directory (default your home folder). Reports exact paths and sizes so the user can inspect them. It never deletes, moves, or modifies files. Do not assume a file or installed app is unused.",
		InputSchema: objectSchema(map[string]interface{}{
			"path":        map[string]interface{}{"type": "string", "description": "Absolute directory to scan (default your home folder)."},
			"min_size_mb": map[string]interface{}{"type": "integer", "description": "Only show files at least this large in MiB (default 100)."},
			"limit":       map[string]interface{}{"type": "integer", "description": "Maximum results, 1-50 (default 20)."},
		}),
	}, handleStorageAudit)

	r.RegisterReadOnly(Tool{
		Name:        "mem_free",
		Description: "Show RAM usage via free -h.",
		InputSchema: objectSchema(map[string]interface{}{}),
	}, handleMemFree)

	r.RegisterReadOnly(Tool{
		Name:        "os_info",
		Description: "Show this machine's OS/distro (from /etc/os-release and uname). Use when asked what OS/version they run.",
		InputSchema: objectSchema(map[string]interface{}{}),
	}, handleOSInfo)

	r.RegisterReadOnly(Tool{
		Name:        "top_processes",
		Description: "Show top CPU/memory processes for the current user. Use for 'what is slowing my PC' / high CPU questions.",
		InputSchema: objectSchema(map[string]interface{}{
			"limit": map[string]interface{}{
				"type": "integer", "description": "How many rows (default 15, max 40).",
			},
			"sort": map[string]interface{}{
				"type": "string", "enum": []string{"cpu", "mem"}, "description": "Sort by cpu or mem (default cpu).",
			},
		}),
	}, handleTopProcesses)

	r.RegisterReadOnly(Tool{
		Name:        "security_scan",
		Description: "Run a read-only ClamAV scan, stream detected file findings to the desktop UI as they arrive, and report the final status. Waits for completion by default; optional timeout is supported. It never deletes or quarantines files.",
		InputSchema: objectSchema(map[string]interface{}{
			"path": map[string]interface{}{
				"type": "string", "description": "Directory or file to scan (default: home). Absolute path only.",
			},
			"timeout_minutes": map[string]interface{}{
				"type": "integer", "minimum": 0, "maximum": 60, "description": "Optional timeout in minutes, 1-60. Default 0 waits for completion; the desktop app has a Stop button.",
			},
		}),
	}, handleSecurityScan)

	r.RegisterReadOnly(Tool{
		Name:        "find_process",
		Description: "List user processes. If name is empty, lists top processes. If name is set, filters by substring.",
		InputSchema: objectSchema(map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string", "description": "Optional process name substring. Empty = top processes.",
			},
		}),
	}, handleFindProcess)

	r.RegisterReadOnly(Tool{
		Name:        "app_install_history",
		Description: "Read local APT/DEB dpkg logs for dated package installation events. Logs may include dependencies and may be incomplete after rotation; no results means the date is unknown. This does not cover Snap, Flatpak, PWAs, or manual installs. Read-only; never modifies packages.",
		InputSchema: objectSchema(map[string]interface{}{
			"query": map[string]interface{}{"type": "string", "description": "Optional package name substring filter."},
			"limit": map[string]interface{}{"type": "integer", "description": "Maximum events, 1-100 (default 40)."},
		}),
	}, handleAppInstallHistory)

	r.RegisterReadOnly(Tool{
		Name:        "list_apps",
		Description: "Search installed apps (apt/snap/flatpak/PWA desktop entries). Use to discover the real name before open_app/close_app.",
		InputSchema: objectSchema(map[string]interface{}{
			"query": map[string]interface{}{
				"type": "string", "description": "Substring to match, e.g. whatsapp, terminal, notes.",
			},
		}),
	}, handleListApps)

	r.Register(Tool{
		Name:        "open_app",
		Description: "Launch an installed app by friendly name. Supports apt, snap, flatpak, Chrome PWAs, and known websites. Resolves .desktop entries — prefer this over guessing binaries.",
		InputSchema: objectSchema(map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string", "description": "App name as the user says it: spotify, Terminal, Notesnook, GitHub, whatsapp…",
			},
		}, "name"),
	}, handleOpenApp)

	r.Register(Tool{
		Name:        "close_app",
		Description: "Close an installed app carefully. Will NOT kill the whole browser to close a website tab. PWAs are closed by app-id; flatpak via flatpak kill; apt/snap by process.",
		InputSchema: objectSchema(map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string", "description": "App name to close.",
			},
		}, "name"),
	}, handleCloseApp)

	r.Register(Tool{
		Name:        "stop_process",
		Description: "Send SIGTERM to a user-owned process by pid or exact process comm. Prefer close_app for applications.",
		InputSchema: objectSchema(map[string]interface{}{
			"name": map[string]interface{}{
				"type": "string", "description": "Exact process name (pgrep -x), e.g. spotify.",
			},
			"pid": map[string]interface{}{
				"type": "integer", "description": "Process id to signal.",
			},
		}),
	}, handleStopProcess)

	r.Register(Tool{
		Name:        "open_uri",
		Description: "Open a file path or URL with the default handler via xdg-open (e.g. https://…, /home/user/file.pdf).",
		InputSchema: objectSchema(map[string]interface{}{
			"uri": map[string]interface{}{
				"type": "string", "description": "https/http URL or absolute file path.",
			},
		}, "uri"),
	}, handleOpenURI)

	r.RegisterReadOnly(Tool{
		Name:        "git_status",
		Description: "Show git status --short --branch for a repo path.",
		InputSchema: objectSchema(map[string]interface{}{
			"dir": map[string]interface{}{"type": "string", "description": "Path to the git working tree."},
		}, "dir"),
	}, handleGitStatus)

	r.RegisterReadOnly(Tool{
		Name:        "git_log",
		Description: "Show recent git commits (oneline).",
		InputSchema: objectSchema(map[string]interface{}{
			"dir": map[string]interface{}{"type": "string"},
			"n":   map[string]interface{}{"type": "integer", "description": "Commit count (default 10, max 50)."},
		}, "dir"),
	}, handleGitLog)

	r.RegisterReadOnly(Tool{
		Name:        "journalctl",
		Description: "Query systemd journal (narrow filters preferred).",
		InputSchema: objectSchema(map[string]interface{}{
			"unit":  map[string]interface{}{"type": "string"},
			"since": map[string]interface{}{"type": "string"},
			"lines": map[string]interface{}{"type": "integer"},
		}),
	}, handleJournalctl)

	r.Register(Tool{
		Name:        "go_test",
		Description: "Run go test in a package directory.",
		InputSchema: objectSchema(map[string]interface{}{
			"dir":      map[string]interface{}{"type": "string"},
			"packages": map[string]interface{}{"type": "string", "description": "Default ./..."},
		}, "dir"),
	}, handleGoTest)
}

func objectSchema(props map[string]interface{}, required ...string) ToolInputSchema {
	return ToolInputSchema{Type: "object", Properties: props, Required: required}
}

func handleInspectPort(args map[string]interface{}) (string, error) {
	port := intArg(args, "port", 0)
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("port must be 1-65535")
	}
	proto := strings.ToLower(strArg(args, "protocol", "tcp"))
	filter := fmt.Sprintf("sport = :%d", port)

	var parts []string
	run := func(flags ...string) error {
		cmd := append([]string{}, flags...)
		cmd = append(cmd, filter)
		res, err := RunCommand(nil, "ss", cmd...)
		if err != nil {
			return err
		}
		parts = append(parts, FormatExec(res))
		return nil
	}

	switch proto {
	case "tcp":
		if err := run("-ltnp"); err != nil {
			return "", err
		}
	case "udp":
		if err := run("-lunp"); err != nil {
			return "", err
		}
	case "all":
		if err := run("-ltnp"); err != nil {
			return "", err
		}
		if err := run("-lunp"); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("protocol must be tcp, udp, or all")
	}
	return strings.Join(parts, "\n"), nil
}

func handleListListeningPorts(args map[string]interface{}) (string, error) {
	ssArgs := []string{"-l", "-n", "-p"}
	switch strings.ToLower(strArg(args, "protocol", "tcp")) {
	case "tcp":
		ssArgs = append(ssArgs, "-t")
	case "udp":
		ssArgs = append(ssArgs, "-u")
	case "all":
		ssArgs = append(ssArgs, "-t", "-u")
	default:
		return "", fmt.Errorf("protocol must be tcp, udp, or all")
	}
	res, err := RunCommand(nil, "ss", ssArgs...)
	if err != nil {
		return "", err
	}
	return FormatExec(res), nil
}

func handleDiskFree(args map[string]interface{}) (string, error) {
	res, err := RunCommand(nil, "df", "-h", "-x", "tmpfs", "-x", "devtmpfs")
	if err != nil {
		return "", err
	}
	return FormatExec(res), nil
}

func handleDiskUsage(args map[string]interface{}) (string, error) {
	path := strings.TrimSpace(strArg(args, "path", "/home"))
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return "", fmt.Errorf("invalid path")
	}
	depth := intArg(args, "depth", 1)
	if depth < 1 {
		depth = 1
	}
	if depth > 2 {
		depth = 2
	}
	res, err := RunCommand(nil, "du", "-x", "-h", fmt.Sprintf("--max-depth=%d", depth), path)
	if err != nil {
		return "", err
	}
	return FormatExec(res), nil
}

func handleMemFree(args map[string]interface{}) (string, error) {
	res, err := RunCommand(nil, "free", "-h")
	if err != nil {
		return "", err
	}
	return FormatExec(res), nil
}

func handleOSInfo(args map[string]interface{}) (string, error) {
	var b strings.Builder
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		b.WriteString("--- /etc/os-release ---\n")
		b.Write(data)
		if !strings.HasSuffix(string(data), "\n") {
			b.WriteByte('\n')
		}
	} else {
		fmt.Fprintf(&b, "os-release: %v\n", err)
	}
	res, err := RunCommand(nil, "uname", "-a")
	if err != nil {
		fmt.Fprintf(&b, "uname: %v\n", err)
	} else {
		b.WriteString(FormatExec(res))
	}
	return b.String(), nil
}

func handleTopProcesses(args map[string]interface{}) (string, error) {
	limit := intArg(args, "limit", 15)
	if limit < 1 {
		limit = 1
	}
	if limit > 40 {
		limit = 40
	}
	sortKey := strings.ToLower(strArg(args, "sort", "cpu"))
	sortFlag := "--sort=-pcpu"
	if sortKey == "mem" {
		sortFlag = "--sort=-pmem"
	}
	uid := strconv.Itoa(os.Getuid())
	res, err := RunCommand(nil, "ps", "-u", uid, "-o", "pid,pcpu,pmem,rss,comm", sortFlag)
	if err != nil {
		return "", err
	}
	lines := strings.Split(res.Stdout, "\n")
	if len(lines) > limit+1 {
		lines = lines[:limit+1]
	}
	res.Stdout = strings.Join(lines, "\n")
	return FormatExec(res), nil
}

func handleSecurityScan(args map[string]interface{}) (string, error) {
	path := strings.TrimSpace(strArg(args, "path", ""))
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = home
	}
	if !filepath.IsAbs(path) || strings.IndexByte(path, 0) >= 0 {
		return "", fmt.Errorf("path must be absolute")
	}
	path = filepath.Clean(path)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("cannot access scan path %s: %w", path, err)
	}
	if _, err := exec.LookPath("clamscan"); err != nil {
		return "clamav: clamscan is not installed or is not on PATH; no security scan was run. Install/configure ClamAV yourself and ask again. This tool does not use sudo.", nil
	}

	ctx := context.Background()
	if requestCtx, ok := args[requestContextArg].(context.Context); ok && requestCtx != nil {
		ctx = requestCtx
	}
	minutes := intArg(args, "timeout_minutes", 0)
	if minutes < 0 {
		minutes = 0
	}
	if minutes > 60 {
		minutes = 60
	}
	var cancel context.CancelFunc
	if minutes > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(minutes)*time.Minute)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	scanID := strconv.FormatInt(time.Now().UnixNano(), 10)
	observer := securityScanObserver(ctx)
	emit := func(kind, message string) {
		if observer != nil {
			observer(SecurityScanEvent{ID: scanID, Type: kind, Path: path, Message: message, At: time.Now()})
		}
	}
	emit("started", "Scanning…")
	findings := 0
	var partial strings.Builder
	processLine := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		if strings.Contains(line, " FOUND") {
			findings++
			emit("finding", line)
		}
	}
	observeOutput := func(chunk []byte) {
		partial.Write(chunk)
		lines := strings.Split(partial.String(), "\n")
		partial.Reset()
		for _, line := range lines[:len(lines)-1] {
			processLine(line)
		}
		partial.WriteString(lines[len(lines)-1])
	}
	command := "clamscan"
	commandArgs := []string{"-r", "-i", "--max-filesize=25M", "--max-scansize=150M", path}
	if _, err := exec.LookPath("stdbuf"); err == nil {
		command = "stdbuf"
		commandArgs = append([]string{"-oL", "-eL", "clamscan"}, commandArgs...)
	}
	res, runErr := RunCommandObserved(ctx, command, observeOutput, commandArgs...)
	processLine(partial.String())
	if res == nil {
		emit("failed", "ClamAV could not start.")
		return "", fmt.Errorf("could not run ClamAV: %w", runErr)
	}

	var b strings.Builder
	b.WriteString("ClamAV scan report (read-only; no files were changed).\n")
	fmt.Fprintf(&b, "Target: %s\n", path)
	status, message := "failed", "ClamAV could not complete the scan. This is not a clean result."
	switch {
	case runErr != nil:
		status = "failed"
	case res.Cancelled:
		status, message = "cancelled", "The scan was stopped before completion; partial findings remain visible above. This is not a clean result."
	case res.TimedOut:
		status, message = "incomplete", fmt.Sprintf("The scan did not finish within %d minute(s); partial findings remain visible above. This is not a clean result.", minutes)
	default:
		switch res.ExitCode {
		case 0:
			status, message = "completed", "Completed with no infected files reported in the scanned scope."
		case 1:
			status, message = "infected", fmt.Sprintf("Completed; ClamAV reported %d infected file(s). Review the findings above. Nothing was removed or quarantined.", findings)
		default:
			status, message = "failed", fmt.Sprintf("ClamAV exited with code %d. This is not a clean result.", res.ExitCode)
		}
	}
	fmt.Fprintf(&b, "Status: %s — %s\n", strings.ToUpper(status), message)
	emit(status, message)
	if res.Stdout != "" {
		fmt.Fprintf(&b, "\n--- ClamAV output ---\n%s\n", res.Stdout)
	}
	if res.Stderr != "" {
		fmt.Fprintf(&b, "\n--- ClamAV diagnostics ---\n%s\n", res.Stderr)
	}
	if runErr != nil {
		fmt.Fprintf(&b, "\nRunner error: %v\n", runErr)
	}
	fmt.Fprintf(&b, "\nFindings streamed: %d\nDuration: %s\n", findings, res.Duration)
	b.WriteString("A completed clean scan is one signal, not a guarantee. Files larger than the configured per-file scan limits may be skipped.")
	return b.String(), nil
}

func handleFindProcess(args map[string]interface{}) (string, error) {
	name := strings.TrimSpace(strArg(args, "name", ""))
	if name == "" {
		return handleTopProcesses(map[string]interface{}{"limit": 20, "sort": "cpu"})
	}
	if err := validateProcName(name); err != nil {
		// allow substrings with spaces stripped already; relax for filter
		if len(name) > 64 {
			return "", err
		}
	}
	uid := strconv.Itoa(os.Getuid())
	res, err := RunCommand(nil, "ps", "-u", uid, "-o", "pid,pcpu,pmem,rss,comm", "--sort=-pcpu")
	if err != nil {
		return "", err
	}
	lower := strings.ToLower(name)
	var kept []string
	for i, line := range strings.Split(res.Stdout, "\n") {
		if i == 0 {
			kept = append(kept, line)
			continue
		}
		if strings.Contains(strings.ToLower(line), lower) {
			kept = append(kept, line)
		}
	}
	if len(kept) <= 1 {
		return fmt.Sprintf("no processes matching %q for current user\n", name), nil
	}
	res.Stdout = strings.Join(kept, "\n")
	return FormatExec(res), nil
}

func handleStopProcess(args map[string]interface{}) (string, error) {
	name := strings.TrimSpace(strArg(args, "name", ""))
	pid := intArg(args, "pid", 0)

	var pids []int
	if pid > 0 {
		pids = []int{pid}
	} else if name != "" {
		if err := validateProcName(name); err != nil {
			return "", err
		}
		uid := strconv.Itoa(os.Getuid())
		res, err := RunCommand(nil, "pgrep", "-u", uid, "-x", name)
		if err != nil {
			return "", err
		}
		if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
			return fmt.Sprintf("no user-owned process named %q\n", name), nil
		}
		for _, line := range strings.Fields(res.Stdout) {
			n, err := strconv.Atoi(line)
			if err != nil {
				continue
			}
			pids = append(pids, n)
		}
	} else {
		return "", fmt.Errorf("provide name or pid")
	}

	var b strings.Builder
	for _, p := range pids {
		if !processOwnedByMe(p) {
			fmt.Fprintf(&b, "pid %d: refused (not owned by current user)\n", p)
			continue
		}
		if err := syscall.Kill(p, syscall.SIGTERM); err != nil {
			fmt.Fprintf(&b, "pid %d: %v\n", p, err)
			continue
		}
		fmt.Fprintf(&b, "pid %d: sent SIGTERM\n", p)
	}
	return b.String(), nil
}

func validateProcName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 64 {
		return fmt.Errorf("name too long")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("invalid process name")
	}
	return nil
}

func processOwnedByMe(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false
	}
	want := strconv.Itoa(os.Getuid())
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			// Uid: real effective saved fs
			return len(fields) >= 2 && fields[1] == want
		}
	}
	return false
}

func handleOpenApp(args map[string]interface{}) (string, error) {
	name := strings.TrimSpace(strArg(args, "name", ""))
	if err := validateAppQuery(name); err != nil {
		return "", err
	}
	app, err := ResolveApp(name)
	if err != nil {
		return fmt.Sprintf("status=fail reason=%q\n", err.Error()), nil
	}
	return LaunchApp(app)
}

func handleCloseApp(args map[string]interface{}) (string, error) {
	name := strings.TrimSpace(strArg(args, "name", ""))
	if err := validateAppQuery(name); err != nil {
		return "", err
	}
	app, err := ResolveApp(name)
	if err != nil {
		return fmt.Sprintf("status=fail reason=%q\n", err.Error()), nil
	}
	return StopApp(app)
}

func handleListApps(args map[string]interface{}) (string, error) {
	q := strings.TrimSpace(strArg(args, "query", ""))
	apps := SearchApps(q, 25)
	if len(apps) == 0 {
		return "no apps matched\n", nil
	}
	var b strings.Builder
	for _, a := range apps {
		fmt.Fprintf(&b, "- %s [%s] id=%s\n", a.Name, a.Kind, a.ID)
	}
	return b.String(), nil
}

func validateAppQuery(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 80 {
		return fmt.Errorf("name too long")
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '-' || r == '_' || r == '.' || r == ' ' {
			continue
		}
		return fmt.Errorf("invalid app name")
	}
	return nil
}

func handleOpenURI(args map[string]interface{}) (string, error) {
	uri := strings.TrimSpace(strArg(args, "uri", ""))
	if uri == "" || strings.IndexByte(uri, 0) >= 0 {
		return "", fmt.Errorf("uri is required")
	}
	lower := strings.ToLower(uri)
	switch {
	case strings.HasPrefix(lower, "https://"), strings.HasPrefix(lower, "http://"):
		// ok
	case strings.HasPrefix(lower, "spotify:"):
		// ok
	case strings.HasPrefix(uri, "/"):
		// absolute path only
	default:
		return "", fmt.Errorf("uri must be http(s), spotify:, or an absolute path")
	}
	if strings.ContainsAny(uri, "\n\r") {
		return "", fmt.Errorf("invalid uri")
	}
	pid, err := StartDetached("xdg-open", uri)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("status=ok kind=uri pid=%d uri=%s\n", pid, uri), nil
}

// resolveDesktopIDs kept for tests; prefer ResolveApp for launches.
func resolveDesktopIDs(name string) []string {
	apps := SearchApps(name, 20)
	var out []string
	for _, a := range apps {
		out = append(out, a.ID)
	}
	if len(out) == 0 {
		out = append(out, name)
	}
	return out
}

func handleGitStatus(args map[string]interface{}) (string, error) {
	dir, err := requireDir(args)
	if err != nil {
		return "", err
	}
	res, err := RunInDir(nil, dir, "git", "status", "--short", "--branch")
	if err != nil {
		return "", err
	}
	return FormatExec(res), nil
}

func handleGitLog(args map[string]interface{}) (string, error) {
	dir, err := requireDir(args)
	if err != nil {
		return "", err
	}
	n := intArg(args, "n", 10)
	if n < 1 {
		n = 1
	}
	if n > 50 {
		n = 50
	}
	res, err := RunInDir(nil, dir, "git", "log", fmt.Sprintf("-%d", n), "--oneline", "--decorate")
	if err != nil {
		return "", err
	}
	return FormatExec(res), nil
}

func handleJournalctl(args map[string]interface{}) (string, error) {
	lines := intArg(args, "lines", 50)
	if lines < 1 {
		lines = 1
	}
	if lines > 200 {
		lines = 200
	}
	cmdArgs := []string{"--no-pager", "-o", "short-iso", "-n", fmt.Sprintf("%d", lines)}
	if u := strArg(args, "unit", ""); u != "" {
		cmdArgs = append(cmdArgs, "-u", u)
	}
	if s := strArg(args, "since", ""); s != "" {
		cmdArgs = append(cmdArgs, "--since", s)
	}
	res, err := RunCommand(nil, "journalctl", cmdArgs...)
	if err != nil {
		return "", err
	}
	return FormatExec(res), nil
}

func handleGoTest(args map[string]interface{}) (string, error) {
	dir, err := requireDir(args)
	if err != nil {
		return "", err
	}
	pkgs := strArg(args, "packages", "./...")
	res, err := RunInDir(nil, dir, "go", "test", pkgs)
	if err != nil {
		return "", err
	}
	return FormatExec(res), nil
}

func requireDir(args map[string]interface{}) (string, error) {
	dir := strings.TrimSpace(strArg(args, "dir", ""))
	if dir == "" {
		return "", fmt.Errorf("dir is required")
	}
	if strings.IndexByte(dir, 0) >= 0 {
		return "", fmt.Errorf("invalid dir")
	}
	return dir, nil
}

func strArg(args map[string]interface{}, key, fallback string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return fallback
	}
	s, ok := v.(string)
	if !ok {
		return fallback
	}
	return s
}

func intArg(args map[string]interface{}, key string, fallback int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return fallback
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return fallback
		}
		return i
	default:
		return fallback
	}
}
