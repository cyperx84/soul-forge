package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cyperx84/soul-forge/internal/config"
)

func TestResultHelpers(t *testing.T) {
	r := Result{Agent: "alpha"}
	if r.HasIssues() {
		t.Fatal("expected no issues")
	}
	if !strings.Contains(r.Format(), "All checks passed") {
		t.Fatalf("unexpected format: %s", r.Format())
	}
	r.Issues = []Issue{{Severity: "info", File: "a", Message: "note"}, {Severity: "warning", Message: "warn"}}
	if !r.HasIssues() {
		t.Fatal("expected issues")
	}
	out := r.Format()
	if !strings.Contains(out, "[warning]") || !strings.Contains(out, "[info]") {
		t.Fatalf("bad format: %s", out)
	}
}

func TestRunAndCheckers(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)

	cfg := &config.Config{OutputDir: "agents", Agents: []config.Agent{{Name: "alpha"}}}
	os.MkdirAll(filepath.Join(".soul-forge"), 0o755)
	profilePath := filepath.Join(".soul-forge", "profile.json")

	agentDir := filepath.Join("agents", "alpha")
	os.MkdirAll(agentDir, 0o755)
	os.WriteFile(filepath.Join(agentDir, "USER.md"), []byte("## Identity\n\nNot specified\n"), 0o644)
	os.WriteFile(filepath.Join(agentDir, "SOUL.md"), []byte("## Agent Identity\n\ncontent\n"), 0o644)
	time.Sleep(1100 * time.Millisecond)
	os.WriteFile(profilePath, []byte("{}"), 0o644)

	results := Run(cfg, cfg.Agents)
	if len(results) != 1 {
		t.Fatalf("results=%d", len(results))
	}
	out := results[0].Format()
	if !strings.Contains(out, "placeholder values") || !strings.Contains(out, "stale") {
		t.Fatalf("format missing issues: %s", out)
	}

	empty := checkEmptySections("## One\n\n## Two\nhello\n")
	if len(empty) != 1 || empty[0] != "One" {
		t.Fatalf("empty=%v", empty)
	}
}

func TestCheckFileMissing(t *testing.T) {
	r := &Result{Agent: "alpha"}
	checkFile(t.TempDir(), "USER.md", nil, r)
	if len(r.Issues) != 1 || r.Issues[0].Severity != "error" {
		t.Fatalf("issues=%+v", r.Issues)
	}
}
