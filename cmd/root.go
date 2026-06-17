package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/cyperx84/soul-forge/internal/config"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "soul-forge",
	Version: config.Version,
	Short:   "Generate SOUL/USER/AGENTS/TOOLS/MEMORY files for agent fleets",
	Long: `soul-forge generates the workspace files an agent fleet needs — SOUL.md (persona),
IDENTITY.md, USER.md, AGENTS.md (operating procedure), TOOLS.md, and MEMORY.md — for
OpenClaw, Hermes, or any harness that reads a soul.md.

It is a deterministic CLI and never calls an LLM provider. The onboarding interview is
driven by your agent harness's own model via the bundled skill; soul-forge handles the
structured parts — questionnaire, dotfiles extraction, generation, and auditing.`,
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
	rootCmd.AddCommand(rubricCmd)
	rootCmd.AddCommand(schemaCmd)
}
