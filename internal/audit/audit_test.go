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
	// USER.md placeholder + staleness + missing optional files + SOUL quality gaps.
	for _, want := range []string{"placeholder values", "stale", "AGENTS.md", "What I Believe"} {
		if !strings.Contains(out, want) {
			t.Fatalf("format missing %q:\n%s", want, out)
		}
	}

	empty := checkEmptySections("## One\n\n## Two\nhello\n")
	if len(empty) != 1 || empty[0] != "One" {
		t.Fatalf("empty=%v", empty)
	}
}

func TestCheckSoulQuality(t *testing.T) {
	// A vague, persona-less SOUL.md should draw several quality warnings.
	weak := "I am bob. I will be helpful and provide comprehensive and thoughtful assistance.\n"
	r := &Result{Agent: "bob"}
	checkSoulQuality(weak, r)
	var msgs string
	for _, i := range r.Issues {
		msgs += i.Message + "\n"
	}
	for _, want := range []string{"be helpful", "comprehensive and thoughtful", "What I Believe", "example exchanges"} {
		if !strings.Contains(msgs, want) {
			t.Errorf("expected quality issue mentioning %q, got:\n%s", want, msgs)
		}
	}

	// A complete soul file should pass the persona-section and example checks.
	strong := "## What I Believe\nx\n## How I Decide\ny\n## What I Won't Do\nz\n## How I Respond\nexample\n"
	r2 := &Result{Agent: "ace"}
	checkSoulQuality(strong, r2)
	for _, i := range r2.Issues {
		if strings.Contains(i.Message, "missing persona section") || strings.Contains(i.Message, "example exchanges") {
			t.Errorf("strong soul file flagged: %s", i.Message)
		}
	}
}

func TestCheckFileMissing(t *testing.T) {
	r := &Result{Agent: "alpha"}
	checkFile(t.TempDir(), fileSpec{"USER.md", true}, nil, r)
	if len(r.Issues) != 1 || r.Issues[0].Severity != "error" {
		t.Fatalf("required missing should error: issues=%+v", r.Issues)
	}
	// Optional files warn instead of erroring when missing.
	r2 := &Result{Agent: "alpha"}
	checkFile(t.TempDir(), fileSpec{"MEMORY.md", false}, nil, r2)
	if len(r2.Issues) != 1 || r2.Issues[0].Severity != "warning" {
		t.Fatalf("optional missing should warn: issues=%+v", r2.Issues)
	}
}
