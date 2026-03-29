package dotfiles

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Info struct {
	Shell    ShellInfo  `json:"shell"`
	Editor   EditorInfo `json:"editor"`
	Git      GitInfo    `json:"git"`
	Tools    []string   `json:"tools"`
	Aliases  []string   `json:"aliases"`
	EnvVars  []string   `json:"env_vars"`
	RawFiles []string   `json:"raw_files"`
}

type ShellInfo struct {
	Shell   string   `json:"shell,omitempty"`
	Plugins []string `json:"plugins,omitempty"`
	Prompt  string   `json:"prompt,omitempty"`
}

type EditorInfo struct {
	Editor     string   `json:"editor,omitempty"`
	Extensions []string `json:"extensions,omitempty"`
}

type GitInfo struct {
	UserName  string   `json:"user_name,omitempty"`
	UserEmail string   `json:"user_email,omitempty"`
	Editor    string   `json:"editor,omitempty"`
	Aliases   []string `json:"aliases,omitempty"`
}

func Scan(repo string) (*Info, error) {
	tmpDir, err := os.MkdirTemp("", "soul-forge-dotfiles-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	cloneURL := fmt.Sprintf("https://github.com/%s.git", repo)
	cmd := exec.Command("git", "clone", "--depth=1", cloneURL, tmpDir)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git clone %s: %w", cloneURL, err)
	}

	return scanDir(tmpDir)
}

// scanDir walks the entire repo tree, detecting config files regardless of depth.
// It handles both root-level dotfiles and stow-style subdirectory layouts
// (e.g. nvim/init.lua, zsh/.zshrc, tmux/tmux.conf).
func scanDir(dir string) (*Info, error) {
	info := &Info{}

	shellFileNames := map[string]bool{
		".zshrc": true, ".bashrc": true, ".bash_profile": true,
		".profile": true, ".zprofile": true, ".zshenv": true,
	}

	// Tool directories: stow-style top-level dirs (e.g. tmux/, ghostty/)
	toolDirNames := map[string]string{
		"kitty":     "kitty",
		"alacritty": "alacritty",
		"wezterm":   "wezterm",
		"tmux":      "tmux",
		"ghostty":   "ghostty",
		"yazi":      "yazi",
		"kanata":    "kanata",
		"aerospace": "aerospace",
	}

	// Tool files: detected by exact filename anywhere in tree
	toolFileNames := map[string]string{
		".tmux.conf":      "tmux",
		"tmux.conf":       "tmux",
		".tool-versions":  "asdf",
		".mise.toml":      "mise",
		"Brewfile":        "homebrew",
		"aerospace.toml":  "aerospace",
	}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			// Detect tools from stow-style directory names
			if tool, ok := toolDirNames[d.Name()]; ok {
				addUnique(&info.Tools, tool)
			}
			// Detect editor from directory names (nvim/, helix/)
			switch d.Name() {
			case "nvim":
				if info.Editor.Editor == "" {
					info.Editor.Editor = "neovim"
				}
			case "helix":
				if info.Editor.Editor == "" {
					info.Editor.Editor = "helix"
				}
			}
			return nil
		}

		rel, _ := filepath.Rel(dir, path)
		name := d.Name()

		// Shell config files — parse for plugins, aliases, env vars
		if shellFileNames[name] {
			if info.Shell.Shell == "" {
				switch name {
				case ".zshrc", ".zprofile", ".zshenv":
					info.Shell.Shell = "zsh"
				case ".bashrc", ".bash_profile":
					info.Shell.Shell = "bash"
				}
			}
			info.RawFiles = append(info.RawFiles, rel)
			if data, err := os.ReadFile(path); err == nil {
				parseShellConfig(string(data), info)
			}
			return nil
		}

		// Git config
		if name == ".gitconfig" {
			info.RawFiles = append(info.RawFiles, rel)
			if data, err := os.ReadFile(path); err == nil {
				parseGitConfig(string(data), info)
			}
			return nil
		}

		// Editor config files
		if isEditorFile(rel, name) {
			info.RawFiles = append(info.RawFiles, rel)
			detectEditor(rel, info)
			return nil
		}

		// VSCode settings
		if name == "settings.json" && strings.Contains(rel, "Code") && strings.Contains(rel, "User") {
			info.RawFiles = append(info.RawFiles, rel)
			if info.Editor.Editor == "" {
				info.Editor.Editor = "vscode"
			}
			return nil
		}

		// Starship config — sets prompt in addition to tool detection
		if name == "starship.toml" {
			info.RawFiles = append(info.RawFiles, rel)
			if info.Shell.Prompt == "" {
				info.Shell.Prompt = "starship"
			}
			addUnique(&info.Tools, "starship")
			return nil
		}

		// Ghostty config file (named "config" inside a ghostty path)
		if name == "config" && strings.Contains(strings.ToLower(rel), "ghostty") {
			info.RawFiles = append(info.RawFiles, rel)
			addUnique(&info.Tools, "ghostty")
			return nil
		}

		// Tool files by exact filename
		if tool, ok := toolFileNames[name]; ok {
			info.RawFiles = append(info.RawFiles, rel)
			addUnique(&info.Tools, tool)
			return nil
		}

		return nil
	})

	return info, err
}

// isEditorFile returns true for known editor config filenames at the expected paths.
func isEditorFile(rel, name string) bool {
	relLower := strings.ToLower(rel)
	if (name == "init.lua" || name == "init.vim") && strings.Contains(relLower, "nvim") {
		return true
	}
	if name == ".vimrc" {
		return true
	}
	if name == "config.toml" && strings.Contains(relLower, "helix") {
		return true
	}
	return false
}

func parseShellConfig(content string, info *Info) {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "zsh") && info.Shell.Shell == "" {
			info.Shell.Shell = "zsh"
		} else if strings.Contains(line, "bash") && info.Shell.Shell == "" {
			info.Shell.Shell = "bash"
		}

		if strings.Contains(line, "oh-my-zsh") {
			addUnique(&info.Shell.Plugins, "oh-my-zsh")
		}
		if strings.Contains(line, "zinit") || strings.Contains(line, "zi ") {
			addUnique(&info.Shell.Plugins, "zinit")
		}
		if strings.Contains(line, "zplug") {
			addUnique(&info.Shell.Plugins, "zplug")
		}
		if strings.Contains(line, "starship") {
			info.Shell.Prompt = "starship"
		}
		if strings.Contains(line, "powerlevel10k") || strings.Contains(line, "p10k") {
			info.Shell.Prompt = "powerlevel10k"
		}

		if strings.HasPrefix(line, "alias ") {
			alias := strings.TrimPrefix(line, "alias ")
			if idx := strings.Index(alias, "="); idx > 0 {
				info.Aliases = append(info.Aliases, alias[:idx])
			}
		}

		if strings.HasPrefix(line, "export ") {
			envVar := strings.TrimPrefix(line, "export ")
			if idx := strings.Index(envVar, "="); idx > 0 {
				varName := envVar[:idx]
				sensitive := []string{"TOKEN", "SECRET", "KEY", "PASSWORD", "PASS", "PWD"}
				isSensitive := false
				for _, s := range sensitive {
					if strings.Contains(strings.ToUpper(varName), s) {
						isSensitive = true
						break
					}
				}
				if !isSensitive {
					info.EnvVars = append(info.EnvVars, varName)
				}
			}
		}
	}
}

func detectEditor(file string, info *Info) {
	if strings.Contains(file, "nvim") {
		info.Editor.Editor = "neovim"
	} else if strings.Contains(file, "vim") {
		info.Editor.Editor = "vim"
	} else if strings.Contains(file, "helix") {
		info.Editor.Editor = "helix"
	}
}

func parseGitConfig(content string, info *Info) {
	lines := strings.Split(content, "\n")
	inAlias := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "[alias]" {
			inAlias = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inAlias = false
			if strings.HasPrefix(line, "[user]") {
				continue
			}
		}
		if strings.HasPrefix(line, "name = ") {
			info.Git.UserName = strings.TrimPrefix(line, "name = ")
		}
		if strings.HasPrefix(line, "email = ") {
			info.Git.UserEmail = strings.TrimPrefix(line, "email = ")
		}
		if inAlias && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			info.Git.Aliases = append(info.Git.Aliases, strings.TrimSpace(parts[0]))
		}
	}
}

func addUnique(slice *[]string, val string) {
	for _, v := range *slice {
		if v == val {
			return
		}
	}
	*slice = append(*slice, val)
}
