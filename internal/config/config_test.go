package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.OutputDir != "agents" {
		t.Fatalf("OutputDir=%q", cfg.OutputDir)
	}
	if len(cfg.Agents) != 1 || cfg.Agents[0].Name != "assistant" {
		t.Fatalf("unexpected default agents: %+v", cfg.Agents)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"ok", Config{OutputDir: "agents", Agents: []Agent{{Name: "a"}}}, ""},
		{"missing output", Config{Agents: []Agent{{Name: "a"}}}, "output_dir must not be empty"},
		{"no agents", Config{OutputDir: "agents"}, "at least one agent"},
		{"empty name", Config{OutputDir: "agents", Agents: []Agent{{Name: ""}}}, "name must not be empty"},
		{"duplicate", Config{OutputDir: "agents", Agents: []Agent{{Name: "a"}, {Name: "a"}}}, "duplicate agent name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.want == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("err=%v want substring %q", err, tt.want)
			}
		})
	}
}

func TestLoadAndWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "soul-forge.yaml")
	cfg := &Config{OutputDir: "out", Dotfiles: "me/dots", Agents: []Agent{{Name: "alpha", Role: "builder", Channel: "ops"}}}
	if err := Write(path, cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "# soul-forge configuration") {
		t.Fatalf("missing header: %s", raw)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.OutputDir != cfg.OutputDir || loaded.Dotfiles != cfg.Dotfiles || loaded.Agents[0].Role != "builder" {
		t.Fatalf("loaded mismatch: %+v", loaded)
	}
}

func TestApplyPersonas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "soul-forge.yaml")
	base := &Config{OutputDir: "agents", Agents: []Agent{
		{Name: "coder", Role: "coding", Channel: "dev"},
		{Name: "ops", Role: "infrastructure"},
	}}
	if err := Write(path, base); err != nil {
		t.Fatal(err)
	}

	seeds := []AgentSeed{
		{Name: "coder", Persona: &Persona{Voice: "dry, precise", Opinions: []string{"delete code over adding it"}}},
		{Name: "researcher", Role: "research", Persona: &Persona{Vibe: "the skeptic"}}, // new agent
	}
	updated, added, err := ApplyPersonas(path, seeds)
	if err != nil {
		t.Fatal(err)
	}
	if updated != 1 || added != 1 {
		t.Fatalf("updated=%d added=%d, want 1/1", updated, added)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Agents) != 3 {
		t.Fatalf("agents=%d, want 3", len(loaded.Agents))
	}
	if loaded.Agents[0].Persona == nil || loaded.Agents[0].Persona.Voice != "dry, precise" {
		t.Fatalf("coder persona not applied: %+v", loaded.Agents[0].Persona)
	}
	if loaded.Agents[0].Role != "coding" { // role unchanged when seed omits it
		t.Fatalf("coder role mutated: %q", loaded.Agents[0].Role)
	}
	newAgent := loaded.Agents[2]
	if newAgent.Name != "researcher" || newAgent.Role != "research" || newAgent.Persona == nil {
		t.Fatalf("new agent wrong: %+v", newAgent)
	}

	// ops had no seed — its (nil) persona must be untouched.
	if loaded.Agents[1].Persona != nil {
		t.Fatalf("ops persona should be nil: %+v", loaded.Agents[1].Persona)
	}

	// A seed with no name is an error.
	if _, _, err := ApplyPersonas(path, []AgentSeed{{Persona: &Persona{}}}); err == nil {
		t.Fatal("expected error for nameless seed")
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	badYAML := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(badYAML, []byte("agents: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(badYAML); err == nil {
		t.Fatal("expected parse error")
	}

	invalid := filepath.Join(dir, "invalid.yaml")
	if err := os.WriteFile(invalid, []byte("output_dir: ''\nagents: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(invalid); err == nil || !strings.Contains(err.Error(), "invalid config") {
		t.Fatalf("got %v", err)
	}
}
