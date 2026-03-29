package interview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildSystemPrompt(t *testing.T) {
	dir := t.TempDir()
	dot := filepath.Join(dir, "dot.json")
	prof := filepath.Join(dir, "profile.json")
	os.WriteFile(dot, []byte(`{"editor":"nvim"}`), 0o644)
	os.WriteFile(prof, []byte(`{"identity":{"name":"Chris"}}`), 0o644)
	s := &Session{Turns: 3, FieldsCaptured: []string{"identity.name"}}
	prompt := BuildSystemPrompt(dot, prof, s, false)
	for _, want := range []string{"Dotfiles Context", "Already Known", "Resume Note", "identity.name", "[COMPLETE]"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
	prompt = BuildSystemPrompt(dot, prof, nil, true)
	if strings.Contains(prompt, "Dotfiles Context") {
		t.Fatalf("dotfiles should be skipped: %s", prompt)
	}
}
