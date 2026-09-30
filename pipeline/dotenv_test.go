package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte("OPENROUTER_API_KEY=fromfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENROUTER_API_KEY", "")
	os.Unsetenv("OPENROUTER_API_KEY")
	if err := LoadDotEnv(p); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("OPENROUTER_API_KEY"); got != "fromfile" {
		t.Fatalf("got %q", got)
	}
}
