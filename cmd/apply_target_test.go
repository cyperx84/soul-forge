package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Target resolution is where an earlier version hardcoded its author's agent and
// machine into the binary. These tests pin the replacement: targets are user data,
// from flags or a file, and the tool ships none of its own.

func resetTargetFlags() {
	applyTarget, applyTargets, applyHost, applyProfile, applyHarness = "", "", "", "", ""
}

func TestResolveTargetAdHoc(t *testing.T) {
	defer resetTargetFlags()
	resetTargetFlags()
	applyHost, applyProfile, applyHarness = "box-a", "scout", "openclaw"

	target, err := resolveTarget()
	if err != nil {
		t.Fatal(err)
	}
	if target.Selector.Host != "box-a" || target.Selector.Profile != "scout" || target.Selector.Harness != "openclaw" {
		t.Errorf("selector not built from flags: %+v", target.Selector)
	}
}

func TestResolveTargetRejectsPartialAdHoc(t *testing.T) {
	defer resetTargetFlags()
	resetTargetFlags()
	applyHost = "box-a" // no profile, no harness

	if _, err := resolveTarget(); err == nil {
		t.Error("partial ad-hoc target must error, not default the missing axes")
	}
}

func TestResolveTargetRejectsNoTarget(t *testing.T) {
	defer resetTargetFlags()
	resetTargetFlags()

	// No default target exists by design: a default would be somebody's setup
	// baked into the tool, and writing it would land in a real workspace.
	if _, err := resolveTarget(); err == nil {
		t.Error("flagless resolution must error, not fall back to a built-in target")
	}
}

func TestResolveTargetFromFile(t *testing.T) {
	defer resetTargetFlags()
	resetTargetFlags()

	dir := t.TempDir()
	path := filepath.Join(dir, "targets.json")
	defs := `[
  {"name": "hub", "host": "box-a", "profile": "scout", "harness": "openclaw"},
  {"name": "cc", "host": "box-a", "profile": "any", "harness": "claude"}
]`
	if err := os.WriteFile(path, []byte(defs), 0o600); err != nil {
		t.Fatal(err)
	}
	applyTargets, applyTarget = path, "cc"

	target, err := resolveTarget()
	if err != nil {
		t.Fatal(err)
	}
	if target.Selector.Harness != "claude" || target.Selector.Profile != "any" {
		t.Errorf("wrong entry resolved: %+v", target.Selector)
	}

	applyTarget = "typo"
	_, err = resolveTarget()
	if err == nil || !strings.Contains(err.Error(), "hub") {
		t.Errorf("unknown name must error listing known names, got: %v", err)
	}
}

func TestResolveTargetRejectsMixedModes(t *testing.T) {
	defer resetTargetFlags()
	resetTargetFlags()
	applyHost, applyTargets = "box-a", "somefile.json"

	if _, err := resolveTarget(); err == nil {
		t.Error("mixing ad-hoc flags with a targets file must error — silent precedence is a guess")
	}
}

func TestLoadTargetsRejectsIncompleteEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "targets.json")
	if err := os.WriteFile(path, []byte(`[{"name": "hub", "host": "box-a"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadTargets(path); err == nil {
		t.Error("an entry missing profile/harness must error — a half-defined target compiles to the wrong scope")
	}
}
