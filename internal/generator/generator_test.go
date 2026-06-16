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

func TestGenerateAllFilesAndMemoryPreserve(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)

	cfg := &config.Config{OutputDir: "agents"}
	prof := &profile.Profile{Identity: profile.Identity{Name: "Chris"}}
	agent := config.Agent{Name: "alpha", Role: "coding"}
	if err := Generate(cfg, prof, agent, false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SOUL.md", "IDENTITY.md", "USER.md", "AGENTS.md", "TOOLS.md", "MEMORY.md"} {
		if _, err := os.Stat(filepath.Join("agents", "alpha", name)); err != nil {
			t.Errorf("expected %s to be generated: %v", name, err)
		}
	}

	// Operational rules belong in AGENTS.md, not SOUL.md (OpenClaw single-responsibility).
	soul, _ := os.ReadFile(filepath.Join("agents", "alpha", "SOUL.md"))
	agents, _ := os.ReadFile(filepath.Join("agents", "alpha", "AGENTS.md"))
	if strings.Contains(string(soul), "Don't commit, push, or delete unless asked") {
		t.Error("operational rule leaked into SOUL.md")
	}
	if !strings.Contains(string(agents), "Don't commit, push, or delete unless asked") {
		t.Error("AGENTS.md missing role operating rules")
	}

	// MEMORY.md must survive a regenerate.
	memPath := filepath.Join("agents", "alpha", "MEMORY.md")
	os.WriteFile(memPath, []byte("learned: Chris hates emoji"), 0o644)
	if err := Generate(cfg, prof, agent, false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(memPath)
	if !strings.Contains(string(data), "hates emoji") {
		t.Errorf("MEMORY.md was clobbered on regenerate: %s", data)
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
