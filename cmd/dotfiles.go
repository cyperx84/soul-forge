package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cyperx84/soul-forge/internal/dotfiles"
	"github.com/spf13/cobra"
)

var dotfilesCmd = &cobra.Command{
	Use:   "dotfiles <user/repo>",
	Short: "Extract tool/env info from a GitHub dotfiles repo",
	Long:  `Clones a GitHub dotfiles repo and scans for shell, editor, git, and tool configs.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDotfiles,
}

func runDotfiles(cmd *cobra.Command, args []string) error {
	repo := args[0]
	fmt.Fprintf(os.Stderr, "Scanning dotfiles repo: %s\n", repo)

	info, err := dotfiles.Scan(repo)
	if err != nil {
		return fmt.Errorf("dotfiles scan failed: %w", err)
	}

	if err := os.MkdirAll(".soul-forge", 0755); err != nil {
		return fmt.Errorf("failed to create .soul-forge directory: %w", err)
	}

	f, err := os.Create(".soul-forge/dotfiles.json")
	if err != nil {
		return fmt.Errorf("failed to create dotfiles.json: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(info); err != nil {
		return fmt.Errorf("failed to write dotfiles.json: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✓ Wrote .soul-forge/dotfiles.json\n")
	enc2 := json.NewEncoder(os.Stdout)
	enc2.SetIndent("", "  ")
	return enc2.Encode(info)
}
