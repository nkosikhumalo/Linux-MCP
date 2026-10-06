package mcp

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type installEvent struct {
	when    time.Time
	pkg     string
	version string
}

// handleAppInstallHistory reads local dpkg logs only. It never runs a package operation.
func handleAppInstallHistory(args map[string]interface{}) (string, error) {
	query := strings.ToLower(strings.TrimSpace(strArg(args, "query", "")))
	limit := intArg(args, "limit", 40)
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	files, err := dpkgLogFiles("/var/log")
	if err != nil {
		return "", err
	}
	var events []installEvent
	var unreadable int
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			unreadable++
			continue
		}
		var reader io.Reader = f
		var gz *gzip.Reader
		if strings.HasSuffix(path, ".gz") {
			gz, err = gzip.NewReader(f)
			if err != nil {
				f.Close()
				unreadable++
				continue
			}
			reader = gz
		}
		sc := bufio.NewScanner(io.LimitReader(reader, 20*1024*1024))
		sc.Buffer(make([]byte, 4096), 1024*1024)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 6 || fields[2] != "install" {
				continue
			}
			t, parseErr := time.ParseInLocation("2006-01-02 15:04:05", fields[0]+" "+fields[1], time.Local)
			if parseErr != nil {
				continue
			}
			pkg := strings.SplitN(fields[3], ":", 2)[0]
			if query != "" && !strings.Contains(strings.ToLower(pkg), query) {
				continue
			}
			version := fields[len(fields)-1]
			events = append(events, installEvent{t, pkg, version})
		}
		if sc.Err() != nil {
			unreadable++
		}
		if gz != nil {
			_ = gz.Close()
		}
		_ = f.Close()
	}
	sort.Slice(events, func(i, j int) bool { return events[i].when.After(events[j].when) })
	if len(events) > limit {
		events = events[:limit]
	}
	var b strings.Builder
	b.WriteString("APT/DEB package installation events from local dpkg logs (source: /var/log/dpkg.log*). These are package events, not a definitive list of apps; dependencies may appear, and old events may be missing after log rotation. Dates are local system time. No package changes were made.\n")
	if len(events) == 0 {
		b.WriteString("No matching install events were found in the available logs. This does not prove the package is absent; its install date may be unknown. Snap, Flatpak, PWA, and manual installs are not covered by these logs.\n")
	}
	for _, e := range events {
		fmt.Fprintf(&b, "- %s — %s %s (APT/DEB package install event)\n", e.when.Format("2006-01-02 15:04:05 MST"), e.pkg, e.version)
	}
	if unreadable > 0 {
		fmt.Fprintf(&b, "%d log file(s) could not be read or parsed.\n", unreadable)
	}
	return b.String(), nil
}

func dpkgLogFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read package logs: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		name := entry.Name()
		if name != "dpkg.log" && !strings.HasPrefix(name, "dpkg.log.") {
			continue
		}
		if entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	return paths, nil
}
