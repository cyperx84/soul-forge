package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set by goreleaser ldflags at build time.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "soul-forge",
	Version: Version,
	Short:   "Compile AI-agent instruction files from a tagged fragment corpus",
	Long: `soul-forge stores every agent instruction as a fragment — one sentence, tagged
with where it applies (host, profile, harness, lifecycle, kind) — and compiles
per-agent, per-machine instruction files from them. A rule is written once and
rendered everywhere it belongs, instead of hand-copied between files and drifting.

Two ways in: 'ingest'/'review'/'merge' migrate existing instruction files into a
corpus; 'onboard' authors one from scratch through a live voice interview. 'apply'
writes compiled targets to disk, dry-run by default.

It is a deterministic CLI and never calls an LLM. Interview briefs are prompts for
whatever voice model the user talks to; the answers come back as text the CLI
parses. The reasoning layer is swappable, the compiler is not.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
