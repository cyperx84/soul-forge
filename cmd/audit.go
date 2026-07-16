package cmd

import (
	"fmt"
	"os"

	"github.com/cyperx84/soul-forge/internal/audit"
	"github.com/cyperx84/soul-forge/internal/compile"
	"github.com/cyperx84/soul-forge/internal/fragment"
	"github.com/spf13/cobra"
)

// audit is the advisory pass: everything real enough to report and not costly
// enough to stop a build. The line matters — compile's invariants have no override
// flag by design, so anything promoted there is unappealable. Audit is where a
// finding lives until it has earned that.

var (
	auditTargets    string
	auditProvenance bool
	auditBudget     int
	auditStrict     bool
)

var auditCmd = &cobra.Command{
	Use:   "audit <corpus.json>",
	Short: "Lint a fragment corpus for rot: duplicates, pinned state, vague rules, bloat",
	Long: `Runs the advisory passes over a corpus (flat array or profile file):

  duplicate-fragment      same text under two ids — the hand-sync surviving in-corpus
  project-state           dated/status lines pinned where they rot
  vague-language          rules that delegate the decision back to the agent
  missing-provenance      harness-behavior claims with no source: citation (--provenance)
  narrow-scope-candidate  'any' tags that only ever reach one target (needs --targets)
  bloat                   files near budget (needs --targets)
  override                a profile chain's child replacing parent doctrine

Exit 0 clean, 1 on warnings (or any finding with --strict), 2 on error.`,
	Args: cobra.ExactArgs(1),
	RunE: runAudit,
}

func init() {
	rootCmd.AddCommand(auditCmd)
	auditCmd.Flags().StringVar(&auditTargets, "targets", "", "targets JSON file; enables narrow-scope and bloat passes")
	auditCmd.Flags().BoolVar(&auditProvenance, "provenance", false, "flag harness-behavior claims lacking a source: citation")
	auditCmd.Flags().IntVar(&auditBudget, "budget", audit.DefaultBudgetBytes, "per-file byte budget the bloat pass warns against")
	auditCmd.Flags().BoolVar(&auditStrict, "strict", false, "exit 1 on info findings too")
}

func runAudit(cmd *cobra.Command, args []string) error {
	frags, overrides, err := loadCorpusWithOverrides(args[0])
	if err != nil {
		return err
	}

	opts := audit.Options{
		Provenance:  auditProvenance,
		BudgetBytes: auditBudget,
		Overrides:   overrides,
	}
	if auditTargets != "" {
		defs, err := loadTargets(auditTargets)
		if err != nil {
			return err
		}
		for _, d := range defs {
			opts.Targets = append(opts.Targets, compile.Target{
				Name:     d.Name,
				Selector: fragment.Selector{Host: d.Host, Profile: d.Profile, Harness: d.Harness},
			})
		}
	}

	findings := audit.Run(frags, opts)
	for _, f := range findings {
		fmt.Fprintln(cmd.OutOrStdout(), f.String())
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%d fragments, %d findings\n", len(frags), len(findings))

	if audit.HasWarnings(findings) || (auditStrict && len(findings) > 0) {
		// Exit 1 via os.Exit rather than an error: findings are the output, not a
		// malfunction, and RunE's error path would print a usage block over them.
		cmd.SilenceUsage = true
		os.Exit(1)
	}
	return nil
}
