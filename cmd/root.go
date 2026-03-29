package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "soul-forge",
	Short: "Scaffold USER.md and SOUL.md files for OpenClaw agent fleets",
	Long: `soul-forge scaffolds and generates USER.md + SOUL.md files for OpenClaw agent fleets.
It handles the structured parts of agent personality/context setup — questionnaire
templates, dotfiles extraction, file generation from structured profiles, and auditing.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		// errAuditFailed already printed its own output; just exit with code 1.
		if errors.Is(err, errAuditFailed) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(questionsCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(dotfilesCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(auditCmd)
}
