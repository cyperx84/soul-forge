package cmd

import (
	"fmt"
	"os"

	"github.com/cyperx84/soul-forge/internal/animation"
	"github.com/cyperx84/soul-forge/internal/config"
	"github.com/spf13/cobra"
)

var (
	noAnimation bool
	initForce   bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize soul-forge in the current directory",
	Long:  `Creates a soul-forge.yaml config file in the current directory.`,
	RunE:  runInit,
}

func init() {
	initCmd.Flags().BoolVar(&noAnimation, "no-animation", false, "Skip the forge animation")
	initCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing soul-forge.yaml")
}

func runInit(cmd *cobra.Command, args []string) error {
	const configFile = "soul-forge.yaml"

	if _, err := os.Stat(configFile); err == nil && !initForce {
		return fmt.Errorf("%s already exists; use --force to overwrite", configFile)
	}

	if !noAnimation && animation.IsTTY() {
		animation.PlayForge()
	}

	if err := os.MkdirAll(".soul-forge", 0755); err != nil {
		return fmt.Errorf("failed to create .soul-forge directory: %w", err)
	}

	cfg := config.Default()
	if err := config.Write(configFile, cfg); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Println("✓ Created soul-forge.yaml")
	fmt.Println("✓ Created .soul-forge/")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit soul-forge.yaml to define your agents")
	fmt.Println("  2. Run: soul-forge questions > answers.md")
	fmt.Println("  3. Fill out your answers")
	fmt.Println("  4. Run: soul-forge import profile.json")
	fmt.Println("  5. Run: soul-forge generate --all")
	return nil
}
