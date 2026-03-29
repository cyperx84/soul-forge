package dotfiles

import (
	"fmt"
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

func scanDir(dir string) (*Info, error) {
	info := &Info{}

	shellFiles := []string{".zshrc", ".bashrc", ".bash_profile", ".profile", ".zprofile", ".zshenv", ".config/zsh/.zshrc"}
	for _, f := range shellFiles {
		path := filepath.Join(dir, f)
		if data, err := os.ReadFile(path); err == nil {
			info.RawFiles = append(info.RawFiles, f)
			parseShellConfig(string(data), info)
		}
	}

	editorFiles := []string{".vimrc", ".config/nvim/init.lua", ".config/nvim/init.vim", ".config/nvim", ".config/helix/config.toml"}
	for _, f := range editorFiles {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); err == nil {
			info.RawFiles = append(info.RawFiles, f)
			detectEditor(f, info)
		}
	}

	vscodePaths := []string{
		".config/Code/User/settings.json",
		"Library/Application Support/Code/User/settings.json",
	}
	for _, f := range vscodePaths {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); err == nil {
			info.RawFiles = append(info.RawFiles, f)
			if info.Editor.Editor == "" {
				info.Editor.Editor = "vscode"
			}
		}
	}

	gitconfig := filepath.Join(dir, ".gitconfig")
	if data, err := os.ReadFile(gitconfig); err == nil {
		info.RawFiles = append(info.RawFiles, ".gitconfig")
		parseGitConfig(string(data), info)
	}

	toolFiles := map[string]string{
		".tmux.conf":             "tmux",
		".config/tmux/tmux.conf": "tmux",
		".config/alacritty":      "alacritty",
		".config/kitty":          "kitty",
		".config/wezterm":        "wezterm",
		".tool-versions":         "asdf",
		".mise.toml":             "mise",
		"Brewfile":               "homebrew",
		".config/starship.toml":  "starship",
	}
	for file, tool := range toolFiles {
		path := filepath.Join(dir, file)
		if _, err := os.Stat(path); err == nil {
			info.RawFiles = append(info.RawFiles, file)
			info.Tools = append(info.Tools, tool)
		}
	}

	return info, nil
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
