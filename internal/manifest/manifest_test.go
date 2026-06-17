package manifest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/config"
)

func TestBuild(t *testing.T) {
	cfg := &config.Config{OutputDir: "agents", Author: "cyperx", License: "MIT"}
	agent := config.Agent{Name: "coder", Role: "software-engineer"} // non-canonical role
	data, err := Build(cfg, agent)
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("soul.json is not valid JSON: %v", err)
	}
	if m.SpecVersion != "0.5" || m.Name != "coder" || m.Version != "0.1.0" {
		t.Fatalf("unexpected manifest core: %+v", m)
	}
	if m.Category != "coding" || len(m.Tags) != 1 || m.Tags[0] != "coding" {
		t.Fatalf("role not canonicalized: %+v", m)
	}
	if m.Author == nil || m.Author.Name != "cyperx" || m.License != "MIT" {
		t.Fatalf("author/license missing: %+v", m)
	}
	if m.Files["soul"] != "SOUL.md" || m.Files["memory"] != "MEMORY.md" {
		t.Fatalf("files map wrong: %+v", m.Files)
	}
	if m.Description == "" {
		t.Fatalf("description should come from role-default vibe")
	}
}

func TestBuildOmitsUnknown(t *testing.T) {
	cfg := &config.Config{OutputDir: "agents"} // no author/license
	data, err := Build(cfg, config.Agent{Name: "x", Role: "general"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\"author\"") {
		t.Fatalf("author should be omitted when unset: %s", data)
	}
	if strings.Contains(string(data), "\"license\"") {
		t.Fatalf("license should be omitted when unset: %s", data)
	}
}
