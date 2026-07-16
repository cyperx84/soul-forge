package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cyperx84/soul-forge/internal/compile"
	"github.com/spf13/cobra"
)

// diff is the drift detector's CLI face. The library existed since the detector
// shipped; production use needs it callable from cron and CI, where the contract
// is the exit code: 0 clean, 1 drift, 2 the comparison itself failed. Those are
// three different situations and a scheduler must be able to tell them apart —
// "drifted" pages a human to reconcile, "failed" pages a human to fix the check.

var (
	diffCorpus  string
	diffTarget  string
	diffTargets string
	diffHost    string
	diffProfile string
	diffHarness string
	diffRoot    string
	diffJSON    bool
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare a compiled target against the files on disk",
	Long: `Compiles a corpus for one target and compares the result against --root,
without writing anything.

Exit code 0 means clean, 1 means drift (a managed file differs or is missing),
2 means the comparison could not be made. Skeleton paths (runtime-owned memory)
and unmanaged leftovers are reported but are never drift — a detector that fires
on things nobody should act on trains its reader to ignore it.`,
	Args: cobra.NoArgs,
	RunE: runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)
	diffCmd.Flags().StringVar(&diffCorpus, "corpus", "", "path to a fragment corpus JSON file (required)")
	diffCmd.Flags().StringVar(&diffTarget, "target", "", "target name to look up in --targets")
	diffCmd.Flags().StringVar(&diffTargets, "targets", "", "path to a targets JSON file (array of {name, host, profile, harness})")
	diffCmd.Flags().StringVar(&diffHost, "host", "", "machine id to compile for (ad-hoc target, with --profile and --harness)")
	diffCmd.Flags().StringVar(&diffProfile, "profile", "", "agent id to compile for (ad-hoc target)")
	diffCmd.Flags().StringVar(&diffHarness, "harness", "", "harness to compile for: openclaw, claude, hermes, or codex (ad-hoc target)")
	diffCmd.Flags().StringVar(&diffRoot, "root", "", "directory to compare against (required)")
	diffCmd.Flags().BoolVar(&diffJSON, "json", false, "emit the report as JSON")
	diffCmd.MarkFlagRequired("corpus")
	diffCmd.MarkFlagRequired("root")
}

func runDiff(cmd *cobra.Command, args []string) error {
	corpus, err := loadCorpus(diffCorpus)
	if err != nil {
		return err
	}
	target, err := resolveTargetFrom(diffTarget, diffTargets, diffHost, diffProfile, diffHarness)
	if err != nil {
		return err
	}

	report, err := compile.Diff(corpus, target, diffRoot)
	if err != nil {
		// RunE's error path exits 1; the comparison failing is a different failure
		// than drift, so it gets its own code.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if diffJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return err
		}
	} else {
		fmt.Printf("target %s vs %s\n\n", report.Target, report.Root)
		for _, f := range report.Files {
			fmt.Printf("%-10s %s\n", f.Status, f.Path)
			for _, l := range f.OnlyCompiled {
				fmt.Printf("  + %s\n", l)
			}
			for _, l := range f.OnlyLive {
				fmt.Printf("  - %s\n", l)
			}
			if f.Reordered {
				fmt.Printf("  ~ same lines, different order\n")
			}
		}
	}

	if report.HasDrift() {
		os.Exit(1)
	}
	return nil
}
