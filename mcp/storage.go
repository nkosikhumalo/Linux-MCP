package mcp

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type storageFile struct {
	path string
	size int64
}

// handleStorageAudit inventories large regular files without modifying them.
func handleStorageAudit(args map[string]interface{}) (string, error) {
	root := strings.TrimSpace(strArg(args, "path", ""))
	if root == "" {
		var err error
		root, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	if !filepath.IsAbs(root) || strings.IndexByte(root, 0) >= 0 {
		return "", fmt.Errorf("path must be an absolute directory")
	}
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("cannot access %s: %w", root, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", root)
	}
	minMB := intArg(args, "min_size_mb", 100)
	if minMB < 1 {
		minMB = 1
	}
	limit := intArg(args, "limit", 20)
	if limit < 1 {
		limit = 1
	}
	if limit > 50 {
		limit = 50
	}
	minBytes := int64(minMB) * 1024 * 1024
	var files []storageFile
	var skipped, visited int
	const maxEntries = 500000
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		visited++
		if visited > maxEntries {
			return filepath.SkipAll
		}
		if walkErr != nil {
			skipped++
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		fi, e := entry.Info()
		if e != nil {
			skipped++
			return nil
		}
		if fi.Size() >= minBytes {
			files = append(files, storageFile{path, fi.Size()})
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].size > files[j].size })
	if len(files) > limit {
		files = files[:limit]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Read-only storage audit of %s (minimum %d MiB).\n", root, minMB)
	if len(files) == 0 {
		b.WriteString("No regular files met the size threshold. Try a lower min_size_mb or use disk_usage for folder totals.\n")
	}
	for _, f := range files {
		fmt.Fprintf(&b, "- %s — %s (%d bytes)\n", f.path, humanBytes(f.size), f.size)
	}
	if skipped > 0 {
		fmt.Fprintf(&b, "Skipped %d unreadable entries.\n", skipped)
	}
	if visited > maxEntries {
		fmt.Fprintf(&b, "Scan stopped at the safety limit of %d entries; results may be incomplete.\n", maxEntries)
	}
	b.WriteString("No files were changed. Explain likely purpose cautiously, suggest verifying that a candidate is still needed and backing it up if appropriate, and ask before cleanup. The user can inspect any reported path with open_uri.")
	return b.String(), nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
