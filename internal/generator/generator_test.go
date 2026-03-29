package generator

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/config"
	"github.com/cyperx84/soul-forge/internal/profile"
)

func TestGenerateWritesFiles(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	cfg := &config.Config{OutputDir: "agents"}
	prof := &profile.Profile{Identity: profile.Identity{Name: "Chris"}, WorkStyle: profile.WorkStyle{Preferences: []string{"speed"}}}
	if err := Generate(cfg, prof, config.Agent{Name: "alpha", Role: "builder", Channel: "ops"}, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"USER.md", "SOUL.md"} {
		data, err := os.ReadFile(filepath.Join("agents", "alpha", name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "Chris") {
			t.Fatalf("%s missing rendered content", name)
		}
	}
}

func TestGenerateDryRunAndValidation(t *testing.T) {
	cfg := &config.Config{OutputDir: "agents"}
	prof := &profile.Profile{}
	out := captureStdout(t, func() {
		if err := Generate(cfg, prof, config.Agent{Name: "alpha", Role: "general"}, true); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "=== USER.md (alpha) ===") {
		t.Fatalf("stdout=%s", out)
	}
	if err := Generate(cfg, prof, config.Agent{Name: "bad/name"}, true); err == nil {
		t.Fatal("expected invalid agent name error")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}
