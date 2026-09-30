// apps.go: resolve & launch Ubuntu apps across apt/snap/flatpak/PWA/web.
package mcp

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// AppKind is how an application is packaged / launched.
type AppKind string

const (
	KindNative  AppKind = "apt"
	KindSnap    AppKind = "snap"
	KindFlatpak AppKind = "flatpak"
	KindPWA     AppKind = "pwa"
	KindWeb     AppKind = "web"
)

// AppEntry is one installable/launchable app discovered from a .desktop file or web alias.
type AppEntry struct {
	Name    string
	ID      string // desktop id without .desktop
	Kind    AppKind
	Exec    []string // trusted argv from .desktop (may include absolute binary path)
	WMClass string
	AppID   string // chrome --app-id=… or flatpak id
	Desktop string // path to .desktop
	URL     string // for KindWeb
}

// well-known sites when no desktop/PWA match exists.
var webAliases = map[string]string{
	"whatsapp":  "https://web.whatsapp.com",
	"linkedin":  "https://www.linkedin.com",
	"gmail":     "https://mail.google.com",
	"youtube":   "https://www.youtube.com",
	"github":    "https://github.com",
	"twitter":   "https://x.com",
	"x":         "https://x.com",
	"facebook":  "https://www.facebook.com",
	"instagram": "https://www.instagram.com",
	"reddit":    "https://www.reddit.com",
	"netflix":   "https://www.netflix.com",
	"outlook":   "https://outlook.live.com",
}

func desktopDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{
		"/var/lib/snapd/desktop/applications",
		"/var/lib/flatpak/exports/share/applications",
		"/usr/share/applications",
		"/usr/local/share/applications",
	}
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local/share/applications"),
			filepath.Join(home, ".local/share/flatpak/exports/share/applications"),
		)
	}
	return dirs
}

// ResolveApp finds the best app match for a user query (name).
func ResolveApp(query string) (*AppEntry, error) {
	q := strings.TrimSpace(strings.ToLower(query))
	q = strings.TrimSuffix(q, ".desktop")
	if q == "" {
		return nil, fmt.Errorf("empty app name")
	}

	var exactName, exactID, partial []*AppEntry
	for _, dir := range desktopDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".desktop") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			app, err := parseDesktopFile(path)
			if err != nil || app == nil || app.Name == "" {
				continue
			}
			nl := strings.ToLower(app.Name)
			idl := strings.ToLower(app.ID)
			switch {
			case nl == q || idl == q:
				if nl == q {
					exactName = append(exactName, app)
				} else {
					exactID = append(exactID, app)
				}
			case strings.Contains(nl, q) || strings.Contains(idl, q) || strings.Contains(strings.ReplaceAll(idl, "_", " "), q):
				partial = append(partial, app)
			}
		}
	}

	pick := func(list []*AppEntry) *AppEntry {
		if len(list) == 0 {
			return nil
		}
		// Prefer non-settings helpers.
		for _, a := range list {
			if !strings.Contains(strings.ToLower(a.Name), "prefer") {
				return a
			}
		}
		return list[0]
	}

	if a := pick(exactName); a != nil {
		return a, nil
	}
	if a := pick(exactID); a != nil {
		return a, nil
	}
	if a := pick(partial); a != nil {
		return a, nil
	}

	// PATH binary (apt/snap symlink).
	if path, err := exec.LookPath(q); err == nil {
		return &AppEntry{
			Name: q,
			ID:   q,
			Kind: kindFromPath(path),
			Exec: []string{path},
		}, nil
	}

	if url, ok := webAliases[q]; ok {
		return &AppEntry{Name: q, ID: q, Kind: KindWeb, URL: url}, nil
	}

	return nil, fmt.Errorf("no installed app matched %q (searched apt/snap/flatpak/PWA desktop entries)", query)
}

func kindFromPath(path string) AppKind {
	if strings.Contains(path, "/snap/") {
		return KindSnap
	}
	return KindNative
}

func parseDesktopFile(path string) (*AppEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	app := &AppEntry{
		ID:      strings.TrimSuffix(filepath.Base(path), ".desktop"),
		Desktop: path,
		Kind:    KindNative,
	}
	if strings.Contains(path, "/snapd/") || strings.Contains(path, "/snap/") {
		app.Kind = KindSnap
	}
	if strings.Contains(path, "flatpak") {
		app.Kind = KindFlatpak
	}

	inEntry := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if !inEntry {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "Type":
			if val != "Application" {
				return nil, nil
			}
		case "NoDisplay", "Hidden":
			if strings.EqualFold(val, "true") {
				return nil, nil
			}
		case "Name":
			if app.Name == "" {
				app.Name = val
			}
		case "Exec":
			if len(app.Exec) == 0 {
				app.Exec = splitDesktopExec(val)
			}
		case "StartupWMClass":
			app.WMClass = val
		case "X-Flatpak":
			app.Kind = KindFlatpak
			app.AppID = val
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if app.Name == "" || len(app.Exec) == 0 {
		return nil, nil
	}

	joined := strings.Join(app.Exec, " ")
	if strings.Contains(joined, "--app-id=") || strings.Contains(joined, "--app=") {
		app.Kind = KindPWA
		for _, a := range app.Exec {
			if strings.HasPrefix(a, "--app-id=") {
				app.AppID = strings.TrimPrefix(a, "--app-id=")
			}
		}
	}
	return app, nil
}

func splitDesktopExec(execLine string) []string {
	// Remove field codes.
	repl := strings.NewReplacer(
		"%f", "", "%F", "", "%u", "", "%U", "",
		"%i", "", "%c", "", "%k", "", "%%", "%",
	)
	execLine = strings.TrimSpace(repl.Replace(execLine))
	var out []string
	var cur strings.Builder
	inQ := false
	for i := 0; i < len(execLine); i++ {
		c := execLine[i]
		switch {
		case c == '"' || c == '\'':
			inQ = !inQ
		case c == ' ' && !inQ:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// LaunchApp starts an app and verifies something actually came up when possible.
func LaunchApp(app *AppEntry) (string, error) {
	if app == nil {
		return "", fmt.Errorf("nil app")
	}

	switch app.Kind {
	case KindWeb:
		pid, err := StartDetached("xdg-open", app.URL)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("status=ok kind=web name=%q url=%s pid=%d note=\"opened in default browser (site/tab — not a standalone package)\"\n",
			app.Name, app.URL, pid), nil

	case KindFlatpak:
		id := app.AppID
		if id == "" {
			id = app.ID
		}
		pid, err := StartDetachedTrusted([]string{"flatpak", "run", id})
		if err != nil {
			// fall through to Exec
			break
		}
		return fmt.Sprintf("status=ok kind=flatpak name=%q id=%s pid=%d\n", app.Name, id, pid), nil

	case KindSnap:
		// Prefer snap binary name before gtk-launch.
		base := app.ID
		if i := strings.Index(base, "_"); i > 0 {
			base = base[:i]
		}
		if path, err := exec.LookPath(base); err == nil {
			pid, err := StartDetachedTrusted([]string{path})
			if err == nil {
				ok := verifyRunning(base, "", 1500*time.Millisecond)
				return fmt.Sprintf("status=ok kind=snap name=%q bin=%s pid=%d verified=%v\n", app.Name, base, pid, ok), nil
			}
		}
	}

	if len(app.Exec) > 0 {
		pid, err := StartDetachedTrusted(app.Exec)
		if err != nil {
			return "", fmt.Errorf("launch %q failed: %w", app.Name, err)
		}
		ok := verifyRunning(app.ID, app.AppID, 1500*time.Millisecond)
		return fmt.Sprintf("status=ok kind=%s name=%q id=%s pid=%d verified=%v\n",
			app.Kind, app.Name, app.ID, pid, ok), nil
	}

	pid, err := StartDetached("gtk-launch", app.ID)
	if err != nil {
		return "", err
	}
	ok := verifyRunning(app.ID, app.AppID, 1500*time.Millisecond)
	return fmt.Sprintf("status=ok kind=%s name=%q id=%s pid=%d via=gtk-launch verified=%v\n",
		app.Kind, app.Name, app.ID, pid, ok), nil
}

// StopApp stops a resolved app carefully (won't nuke Chrome to close a website).
func StopApp(app *AppEntry) (string, error) {
	if app == nil {
		return "", fmt.Errorf("nil app")
	}

	switch app.Kind {
	case KindWeb:
		return fmt.Sprintf("status=fail kind=web name=%q reason=\"this is a website/tab in the browser — close that tab yourself; I will not kill the whole browser\"\n", app.Name), nil

	case KindPWA:
		if app.AppID == "" {
			return fmt.Sprintf("status=fail kind=pwa name=%q reason=\"missing app-id\"\n", app.Name), nil
		}
		pids := pidsMatchingCmdline(app.AppID)
		if len(pids) == 0 {
			return fmt.Sprintf("status=fail kind=pwa name=%q reason=\"PWA not running\"\n", app.Name), nil
		}
		n := signalPIDs(pids)
		return fmt.Sprintf("status=ok kind=pwa name=%q signaled=%d\n", app.Name, n), nil

	case KindFlatpak:
		id := app.AppID
		if id == "" {
			id = app.ID
		}
		res, err := RunCommand(nil, "flatpak", "kill", id)
		if err != nil {
			return "", err
		}
		if res.ExitCode != 0 {
			return fmt.Sprintf("status=fail kind=flatpak name=%q detail=%q\n", app.Name, res.Stderr), nil
		}
		return fmt.Sprintf("status=ok kind=flatpak name=%q id=%s\n", app.Name, id), nil
	}

	// apt / snap: match by binary basename from Exec, not vague substrings.
	bin := ""
	if len(app.Exec) > 0 {
		bin = filepath.Base(app.Exec[0])
		// snap wrapper
		if bin == "snap" && len(app.Exec) >= 3 && app.Exec[1] == "run" {
			bin = app.Exec[2]
		}
		if bin == "flatpak" {
			bin = ""
		}
	}
	if bin == "" {
		bin = app.ID
		if i := strings.Index(bin, "_"); i > 0 {
			bin = bin[:i]
		}
	}

	// gnome-terminal quirk
	candidates := []string{bin}
	if strings.Contains(strings.ToLower(app.ID), "terminal") || strings.Contains(strings.ToLower(app.Name), "terminal") {
		candidates = append(candidates, "gnome-terminal-server", "gnome-terminal", "kgx", "ptyxis")
	}

	var all []int
	for _, c := range candidates {
		all = append(all, pidsExactComm(c)...)
		all = append(all, pidsMatchingCmdline("/"+c)...)
	}
	all = uniqueInts(all)
	if len(all) == 0 {
		return fmt.Sprintf("status=fail kind=%s name=%q reason=\"not running\"\n", app.Kind, app.Name), nil
	}
	n := signalPIDs(all)
	return fmt.Sprintf("status=ok kind=%s name=%q signaled=%d\n", app.Kind, app.Name, n), nil
}

func verifyRunning(id, appID string, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if appID != "" && len(pidsMatchingCmdline(appID)) > 0 {
			return true
		}
		base := id
		if i := strings.Index(base, "_"); i > 0 {
			base = base[:i]
		}
		if len(pidsExactComm(base)) > 0 || len(pidsMatchingCmdline(base)) > 0 {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

func pidsExactComm(comm string) []int {
	comm = strings.TrimSpace(comm)
	if comm == "" {
		return nil
	}
	uid := fmt.Sprintf("%d", os.Getuid())
	res, err := RunCommand(nil, "pgrep", "-u", uid, "-x", truncateComm(comm))
	if err != nil || res.ExitCode != 0 {
		return nil
	}
	return parsePIDList(res.Stdout)
}

func truncateComm(s string) string {
	// Linux COMM is 15 chars.
	if len(s) > 15 {
		return s[:15]
	}
	return s
}

func pidsMatchingCmdline(substr string) []int {
	substr = strings.TrimSpace(substr)
	if substr == "" {
		return nil
	}
	uid := os.Getuid()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []int
	for _, e := range entries {
		pid, err := strconvAtoi(e.Name())
		if err != nil {
			continue
		}
		if !processOwnedByMe(pid) {
			continue
		}
		// skip self
		if pid == os.Getpid() {
			continue
		}
		cmdline, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		cmd := strings.ReplaceAll(string(cmdline), "\x00", " ")
		if strings.Contains(cmd, substr) {
			// extra: must still be our uid (already checked)
			_ = uid
			out = append(out, pid)
		}
	}
	return out
}

func signalPIDs(pids []int) int {
	n := 0
	for _, p := range pids {
		if !processOwnedByMe(p) {
			continue
		}
		if err := syscall.Kill(p, syscall.SIGTERM); err == nil {
			n++
		}
	}
	return n
}

func parsePIDList(s string) []int {
	var out []int
	for _, f := range strings.Fields(s) {
		n, err := strconvAtoi(f)
		if err == nil {
			out = append(out, n)
		}
	}
	return out
}

func uniqueInts(in []int) []int {
	seen := map[int]bool{}
	var out []int
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func strconvAtoi(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not int")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// SearchApps returns up to limit matches for a query (for list_apps tool).
func SearchApps(query string, limit int) []*AppEntry {
	if limit <= 0 {
		limit = 20
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var out []*AppEntry
	seen := map[string]bool{}
	for _, dir := range desktopDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if len(out) >= limit {
				return out
			}
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".desktop") {
				continue
			}
			app, err := parseDesktopFile(filepath.Join(dir, e.Name()))
			if err != nil || app == nil {
				continue
			}
			key := string(app.Kind) + ":" + app.ID
			if seen[key] {
				continue
			}
			if q == "" || strings.Contains(strings.ToLower(app.Name), q) || strings.Contains(strings.ToLower(app.ID), q) {
				seen[key] = true
				out = append(out, app)
			}
		}
	}
	return out
}
