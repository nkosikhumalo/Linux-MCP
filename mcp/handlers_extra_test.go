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

func TestFormatDiskUsageResultRanksNestedPathsAndReportsPartialScans(t *testing.T) {
	complete := &ExecResult{
		Stdout:   "1000\t/home/user\x00200\t/home/user/.cache\x00500\t/home/user/Downloads\x00",
		ExitCode: 0,
	}
	got := formatDiskUsageResult(complete, "/home/user", 4)
	if !strings.Contains(got, "Scan status: complete") || !strings.Contains(got, "1000B\t/home/user") {
		t.Fatalf("missing complete status or root total: %s", got)
	}
	cache := strings.Index(got, "/home/user/.cache")
	downloads := strings.Index(got, "/home/user/Downloads")
	if cache < 0 || downloads < 0 || downloads > cache {
		t.Fatalf("directory totals are not ranked largest first: %s", got)
	}

	partial := &ExecResult{Stdout: complete.Stdout, ExitCode: 1, Stderr: "du: cannot read directory"}
	got = formatDiskUsageResult(partial, "/home/user", 4)
	if !strings.Contains(got, "Scan status: partial or uncertain") || !strings.Contains(got, "Treat listed sizes as partial") {
		t.Fatalf("partial scan status missing: %s", got)
	}
}
