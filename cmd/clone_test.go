package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

func writeBaseProfile(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "prime.json")
	content := `{
		"name": "prime",
		"fragments": [
			{"id":"red-line","text":"Never exfiltrate private data.","host":"any","profile":"any","harness":"any","lifecycle":"authored","kind":"rule"},
			{"id":"disk","text":"Disk is roomy.","host":"any","profile":"any","harness":"any","lifecycle":"authored","kind":"fact"}
		]
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func resetCloneFlags() {
	cloneAs, cloneOut, cloneSets, cloneRetags, cloneForce = "", "", nil, nil, false
}

func TestCloneInheritsWithoutCopying(t *testing.T) {
	resetCloneFlags()
	dir := t.TempDir()
	base := writeBaseProfile(t, dir)
	cloneAs = "m1"

	if err := runClone(cloneCmd, []string{base}); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(dir, "m1.json")
	frags, overrides, err := mustResolve(t, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 0 {
		t.Fatalf("empty clone must not override, got %v", overrides)
	}
	if len(frags) != 2 {
		t.Fatalf("clone must inherit both base fragments, got %d", len(frags))
	}
	// Inherited, not copied: the child file itself must hold zero fragments, or a
	// base fix would never reach it.
	c, err := fragment.LoadProfile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Fragments) != 0 {
		t.Fatalf("clone copied %d fragments into the child file; inheritance must carry them", len(c.Fragments))
	}
}

func TestCloneSetOverridesInPlace(t *testing.T) {
	resetCloneFlags()
	dir := t.TempDir()
	base := writeBaseProfile(t, dir)
	cloneAs = "m4"
	cloneSets = []string{"disk=Disk chronically tight, mind footprints."}

	if err := runClone(cloneCmd, []string{base}); err != nil {
		t.Fatal(err)
	}
	frags, overrides, err := mustResolve(t, filepath.Join(dir, "m4.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 1 || overrides[0].ID != "disk" {
		t.Fatalf("want one recorded disk override, got %v", overrides)
	}
	// Override keeps the parent's position and scope tags — a redefinition, not a
	// new fragment.
	if frags[1].ID != "disk" || frags[1].Text != "Disk chronically tight, mind footprints." || frags[1].Kind != fragment.KindFact {
		t.Fatalf("override must replace text in place with parent tags, got %+v", frags[1])
	}
}

func TestCloneSetAndRetagCombineIntoOneOverride(t *testing.T) {
	// The real provisioning case: a machine clone replaces a fact's text with
	// machine-specific content, which the inherited host:any would misroute into
	// the portable rules file. Set and retag on one id must produce ONE child
	// fragment — two would be duplicate IDs, which compile rejects.
	resetCloneFlags()
	dir := t.TempDir()
	base := writeBaseProfile(t, dir)
	cloneAs = "m4"
	cloneSets = []string{"disk=Disk chronically tight."}
	cloneRetags = []string{"disk=host:m4-mini"}

	if err := runClone(cloneCmd, []string{base}); err != nil {
		t.Fatal(err)
	}
	c, err := fragment.LoadProfile(filepath.Join(dir, "m4.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Fragments) != 1 {
		t.Fatalf("set+retag on one id must emit one child fragment, got %d", len(c.Fragments))
	}
	got := c.Fragments[0]
	if got.Text != "Disk chronically tight." || got.Host != "m4-mini" || got.Kind != fragment.KindFact {
		t.Fatalf("child must carry new text AND new host with parent kind, got %+v", got)
	}
}

func TestCloneRetagRejectsClosedAxes(t *testing.T) {
	resetCloneFlags()
	dir := t.TempDir()
	base := writeBaseProfile(t, dir)
	cloneAs = "m4"
	cloneRetags = []string{"disk=kind:rule"}
	if err := runClone(cloneCmd, []string{base}); err == nil {
		t.Fatal("retagging kind must error: a different kind is a different fragment")
	}
	resetCloneFlags()
	cloneAs = "m4"
	cloneRetags = []string{"disk=harness:not-a-harness"}
	if err := runClone(cloneCmd, []string{base}); err == nil {
		t.Fatal("retag must validate the new value against the axis's closed set")
	}
}

func TestCloneSetUnknownIDErrors(t *testing.T) {
	resetCloneFlags()
	dir := t.TempDir()
	base := writeBaseProfile(t, dir)
	cloneAs = "m1"
	cloneSets = []string{"no-such-fragment=text"}
	if err := runClone(cloneCmd, []string{base}); err == nil {
		t.Fatal("overriding a nonexistent fragment is a typo and must error")
	}
}

func TestCloneRefusesExistingOutput(t *testing.T) {
	resetCloneFlags()
	dir := t.TempDir()
	base := writeBaseProfile(t, dir)
	existing := filepath.Join(dir, "m1.json")
	if err := os.WriteFile(existing, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	cloneAs = "m1"
	if err := runClone(cloneCmd, []string{base}); err == nil {
		t.Fatal("clone must refuse to overwrite without --force")
	}
	b, _ := os.ReadFile(existing)
	if string(b) != "precious" {
		t.Fatal("refusal must leave the existing file untouched")
	}
}

func TestCloneRejectsNameCollision(t *testing.T) {
	resetCloneFlags()
	dir := t.TempDir()
	base := writeBaseProfile(t, dir)
	cloneAs = "prime"
	if err := runClone(cloneCmd, []string{base}); err == nil {
		t.Fatal("--as matching the base name must error: duplicate names read as a chain cycle")
	}
}

func TestClonedChainSurvivesRelocation(t *testing.T) {
	resetCloneFlags()
	dir := t.TempDir()
	base := writeBaseProfile(t, dir)
	cloneAs = "m1"
	if err := runClone(cloneCmd, []string{base}); err != nil {
		t.Fatal(err)
	}

	// Move the whole tree — the provisioning story is "copy this directory to the
	// new box". An absolute extends path would break exactly here.
	moved := filepath.Join(t.TempDir(), "kit")
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	frags, _, err := mustResolve(t, filepath.Join(moved, "m1.json"))
	if err != nil {
		t.Fatalf("relocated profile tree must still resolve: %v", err)
	}
	if len(frags) != 2 {
		t.Fatalf("relocated chain lost fragments: got %d", len(frags))
	}
}

// mustResolve loads a profile via the same loader every compile-path command uses,
// so these tests cover the sniffing seam too.
func mustResolve(t *testing.T, path string) ([]fragment.Fragment, []fragment.Override, error) {
	t.Helper()
	return loadCorpusWithOverrides(path)
}
