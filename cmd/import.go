package cmd

import (
	"fmt"

	"github.com/cyperx84/soul-forge/internal/profile"
	"github.com/spf13/cobra"
)

var importMerge bool

var importCmd = &cobra.Command{
	Use:   "import <profile.json>",
	Short: "Import a structured profile JSON",
	Long:  `Imports a structured profile JSON into .soul-forge/profile.json.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runImport,
}

func init() {
	importCmd.Flags().BoolVar(&importMerge, "merge", false, "Merge with existing profile instead of overwriting")
}

func runImport(cmd *cobra.Command, args []string) error {
	src := args[0]
	dst := ".soul-forge/profile.json"

	if importMerge {
		if err := profile.Merge(src, dst); err != nil {
			return fmt.Errorf("merge failed: %w", err)
		}
		fmt.Printf("✓ Merged %s into %s\n", src, dst)
	} else {
		if err := profile.Import(src, dst); err != nil {
			return fmt.Errorf("import failed: %w", err)
		}
		fmt.Printf("✓ Imported %s → %s\n", src, dst)
	}
	return nil
}
