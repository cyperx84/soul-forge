package interview

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionLifecycle(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)

	s := NewSession("openai", "gpt")
	s.AppendMessage("assistant", "hi")
	s.AppendMessage("user", "yo")
	if s.Turns != 1 {
		t.Fatalf("turns=%d", s.Turns)
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSession()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Provider != "openai" || len(loaded.Messages) != 2 {
		t.Fatalf("loaded=%+v", loaded)
	}
	msgs := loaded.ToLLMMessages()
	if len(msgs) != 2 || msgs[1].Role != "user" {
		t.Fatalf("msgs=%+v", msgs)
	}
	if _, err := os.Stat(filepath.Join(".soul-forge", "session.json")); err != nil {
		t.Fatal(err)
	}
}
