package pipeline

import (
	"bufio"
	"os"
	"strings"
)

// LoadDotEnv reads KEY=VALUE pairs from path into the process environment.
// Existing env vars are not overwritten. Missing file is a no-op.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}
	return sc.Err()
}

// LoadDotEnvDefault tries ./.env then walks up a few parents (handy for wails/bin).
func LoadDotEnvDefault() {
	candidates := []string{".env"}
	dir, err := os.Getwd()
	if err == nil {
		for i := 0; i < 4; i++ {
			candidates = append(candidates, dir+string(os.PathSeparator)+".env")
			parent := strings.TrimSuffix(dir, string(os.PathSeparator))
			idx := strings.LastIndex(parent, string(os.PathSeparator))
			if idx <= 0 {
				break
			}
			dir = parent[:idx]
		}
	}
	seen := map[string]bool{}
	for _, p := range candidates {
		if seen[p] {
			continue
		}
		seen[p] = true
		_ = LoadDotEnv(p)
	}
}
