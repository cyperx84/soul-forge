package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFieldsCaptured(t *testing.T) {
	p := &Profile{
		Identity: Identity{Name: "Chris", Goals: []string{"ship"}, Timezone: "AEST"},
		WorkStyle: WorkStyle{Tools: []string{"go"}, OutputPreferences: map[string]string{"style": "diff"}},
		Environment: Environment{OS: "macOS", Aliases: []string{"gs"}},
	}
	got := p.FieldsCaptured()
	want := []string{"identity.name", "identity.goals", "identity.timezone", "work_style.tools", "work_style.output_preferences", "environment.os", "environment.aliases"}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Fatalf("missing %s in %v", w, got)
		}
	}
}

func TestLoadImportAndMerge(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)

	src := filepath.Join(dir, "src.json")
	base := Profile{
		Identity:    Identity{Name: "Chris", Goals: []string{"build"}},
		WorkStyle:   WorkStyle{OutputPreferences: map[string]string{"format": "diff"}},
		Environment: Environment{OS: "macOS"},
		UpdatedAt:   "t1",
	}
	writeJSON(t, src, base)

	dst := filepath.Join(dir, ".soul-forge", "profile.json")
	if err := Import(src, dst); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dst)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Identity.Name != "Chris" || loaded.Environment.OS != "macOS" {
		t.Fatalf("bad import: %+v", loaded)
	}

	overlaySrc := filepath.Join(dir, "overlay.json")
	overlay := Profile{
		Identity:    Identity{Role: "builder"},
		WorkStyle:   WorkStyle{OutputPreferences: map[string]string{"verbosity": "low"}},
		Environment: Environment{Shell: "zsh"},
		UpdatedAt:   "t2",
	}
	writeJSON(t, overlaySrc, overlay)
	if err := Merge(overlaySrc, dst); err != nil {
		t.Fatal(err)
	}
	merged, err := Load(dst)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Identity.Name != "Chris" || merged.Identity.Role != "builder" || merged.WorkStyle.OutputPreferences["format"] != "diff" || merged.WorkStyle.OutputPreferences["verbosity"] != "low" || merged.UpdatedAt != "t2" {
		t.Fatalf("bad merge: %+v", merged)
	}
}

func TestLoadImportMergeErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := Load(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("expected load error")
	}
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{"), 0o644)
	if err := Import(bad, filepath.Join(dir, "out.json")); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("got %v", err)
	}
	if err := Merge(bad, filepath.Join(dir, "out.json")); err == nil || !strings.Contains(err.Error(), "read src") {
		t.Fatalf("got %v", err)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
