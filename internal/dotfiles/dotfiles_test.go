package dotfiles

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestScanDir(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".zshrc"), "source oh-my-zsh\neval \"$(starship init zsh)\"\nalias gs='git status'\nexport PATH=/bin\nexport API_KEY=secret\n")
	mustWrite(t, filepath.Join(dir, ".config", "nvim", "init.lua"), "")
	mustWrite(t, filepath.Join(dir, ".gitconfig"), "[user]\nname = Chris\nemail = c@example.com\n[alias]\n  co = checkout\n")
	os.MkdirAll(filepath.Join(dir, ".config", "kitty"), 0o755)
	mustWrite(t, filepath.Join(dir, "Brewfile"), "")

	info, err := scanDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Shell.Shell != "zsh" || info.Shell.Prompt != "starship" || !slices.Contains(info.Shell.Plugins, "oh-my-zsh") {
		t.Fatalf("shell=%+v", info.Shell)
	}
	if info.Editor.Editor != "neovim" {
		t.Fatalf("editor=%+v", info.Editor)
	}
	if info.Git.UserName != "Chris" || !slices.Contains(info.Git.Aliases, "co") {
		t.Fatalf("git=%+v", info.Git)
	}
	if !slices.Contains(info.Aliases, "gs") || !slices.Contains(info.EnvVars, "PATH") || slices.Contains(info.EnvVars, "API_KEY") {
		t.Fatalf("aliases/env=%+v %+v", info.Aliases, info.EnvVars)
	}
	if !slices.Contains(info.Tools, "kitty") || !slices.Contains(info.Tools, "homebrew") {
		t.Fatalf("tools=%v", info.Tools)
	}
}

func TestHelpers(t *testing.T) {
	info := &Info{}
	parseShellConfig("bash\nsource zplug\nzi light foo\np10k configure\n", info)
	if info.Shell.Shell != "bash" || info.Shell.Prompt != "powerlevel10k" || !slices.Contains(info.Shell.Plugins, "zplug") || !slices.Contains(info.Shell.Plugins, "zinit") {
		t.Fatalf("shell=%+v", info.Shell)
	}
	detectEditor(".config/helix/config.toml", info)
	if info.Editor.Editor != "helix" {
		t.Fatalf("editor=%+v", info.Editor)
	}
	parseGitConfig("[alias]\n  st = status\n", info)
	if !slices.Contains(info.Git.Aliases, "st") {
		t.Fatalf("aliases=%v", info.Git.Aliases)
	}
	vals := []string{"a"}
	addUnique(&vals, "a")
	addUnique(&vals, "b")
	if len(vals) != 2 {
		t.Fatalf("vals=%v", vals)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
