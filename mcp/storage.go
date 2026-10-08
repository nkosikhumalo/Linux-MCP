package mcp

import (
	"container/heap"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

type storageFile struct {
	path string
	size int64
}

// storageMinHeap keeps the smallest retained file at its root, so memory use is O(limit).
type storageMinHeap []storageFile

func (h storageMinHeap) Len() int            { return len(h) }
func (h storageMinHeap) Less(i, j int) bool  { return h[i].size < h[j].size }
func (h storageMinHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *storageMinHeap) Push(x interface{}) { *h = append(*h, x.(storageFile)) }
func (h *storageMinHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// handleStorageAudit inventories the largest regular files without modifying them.
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
	rootInfo, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("cannot access %s: %w", root, err)
	}
	if !rootInfo.IsDir() {
		return "", fmt.Errorf("path is not a directory: %s", root)
	}
	rootDev, sameDevice := deviceID(rootInfo)

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
	query := strings.ToLower(strings.TrimSpace(strArg(args, "query", "")))
	if query != "" {
		// Filename searches must find small files too; size is reported, never used to hide matches.
		minBytes = 0
	}

	// Normalize once, then use a hash set for constant-time extension checks.
	extArg := strArg(args, "extensions", "")
	extensions := make(map[string]struct{})
	for _, ext := range strings.Split(extArg, ",") {
		ext = strings.ToLower(strings.TrimSpace(ext))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extensions[ext] = struct{}{}
	}

	const maxEntries = 500000
	virtualDirs := map[string]bool{"/proc": true, "/sys": true, "/dev": true, "/run": true}
	retained := &storageMinHeap{}
	heap.Init(retained)
	var skipped, visited int
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		visited++
		if query == "" && visited > maxEntries {
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
		if entry.IsDir() {
			clean := filepath.Clean(path)
			if virtualDirs[clean] {
				return filepath.SkipDir
			}
			if sameDevice {
				if info, e := entry.Info(); e == nil {
					if dev, ok := deviceID(info); ok && dev != rootDev {
						return filepath.SkipDir
					}
				}
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if query != "" && !strings.Contains(strings.ToLower(entry.Name()), query) {
			return nil
		}
		if len(extensions) > 0 {
			if _, ok := extensions[strings.ToLower(filepath.Ext(entry.Name()))]; !ok {
				return nil
			}
		}
		fi, e := entry.Info()
		if e != nil {
			skipped++
			return nil
		}
		if fi.Size() < minBytes {
			return nil
		}
		candidate := storageFile{path: path, size: fi.Size()}
		if retained.Len() < limit {
			heap.Push(retained, candidate)
		} else if candidate.size > (*retained)[0].size {
			heap.Pop(retained)
			heap.Push(retained, candidate)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	files := []storageFile(*retained)
	sort.Slice(files, func(i, j int) bool { return files[i].size > files[j].size })
	var b strings.Builder
	fmt.Fprintf(&b, "Read-only storage audit of %s.\n", root)
	if query != "" {
		fmt.Fprintf(&b, "Filename contains %q; no minimum size applied.\n", query)
	} else {
		fmt.Fprintf(&b, "Minimum size: %d MiB.\n", minMB)
	}
	if len(extensions) > 0 {
		fmt.Fprintf(&b, "Extensions: %s\n", strings.Join(sortedKeys(extensions), ", "))
	}
	if len(files) == 0 {
		if query != "" {
			fmt.Fprintf(&b, "No regular file names containing %q were found in the scanned scope.\n", query)
		} else {
			b.WriteString("No regular files met the size threshold. Try a lower min_size_mb or use disk_usage for folder totals.\n")
		}
	}
	for _, f := range files {
		fmt.Fprintf(&b, "- %s — %s (%d bytes)\n", f.path, humanBytes(f.size), f.size)
		fmt.Fprintf(&b, "  [Show in Files](reveal://%s)\n", url.QueryEscape(f.path))
	}
	if skipped > 0 {
		fmt.Fprintf(&b, "Skipped %d unreadable entries.\n", skipped)
	}
	if query == "" && visited > maxEntries {
		fmt.Fprintf(&b, "Scan stopped at the safety limit of %d entries; results are incomplete and absence of a match is not conclusive.\n", maxEntries)
	} else if skipped > 0 {
		b.WriteString("Traversal reached the end of the accessible scope; unreadable entries were omitted.\n")
	} else {
		b.WriteString("Scan completed within the requested scope.\n")
	}
	b.WriteString("Symlinks and virtual directories (/proc, /sys, /dev, /run) are skipped; mounted filesystems on another device are not entered. Use disk_usage to identify large folders and their contents. No files were changed. The user decides whether anything should be removed.")
	return b.String(), nil
}

func deviceID(info fs.FileInfo) (uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(stat.Dev), true
}

func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
