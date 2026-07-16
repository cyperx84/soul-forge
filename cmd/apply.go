package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cyperx84/soul-forge/internal/compile"
	"github.com/cyperx84/soul-forge/internal/fragment"
	"github.com/spf13/cobra"
)

// apply is the only command in this tool that writes to files an agent reads every
// session. Three things shape its surface, and each one is a decision rather than a
// convention:
//
// Dry run is the default and --commit is the only way to write. The inverse (write by
// default, --dry-run to preview) is the common CLI shape and it is wrong here: the
// cost of an accidental preview is nothing, and the cost of an accidental overwrite is
// a hand-written rulebook.
//
// Backups are on unless turned off. --no-backup exists because applying into a clean
// git tree already has a better backup than this tool can make, and a second one is
// noise the operator learns to ignore.
//
// An overwrite is refused without --force. apply cannot tell a stale compile from a
// hand edit made five minutes ago — both are just "disk differs". So the destructive
// case stops and says what it would replace, rather than resolving the ambiguity in
// its own favour.

var (
	applyCorpus   string
	applyTarget   string
	applyTargets  string
	applyHost     string
	applyProfile  string
	applyHarness  string
	applyRoot     string
	applyCommit   bool
	applyForce    bool
	applyNoBackup bool
	applyBackupTo string
	applyJSON     bool
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Write a compiled target to disk",
	Long: `Compiles a corpus for one target and writes the result under --root.

Dry run by default: prints what it would do and exits. --commit writes.

Never touches MEMORY.md contents (the runtime owns them), never touches files the
compile emits nothing for, and refuses to overwrite a file whose content differs
unless --force — a difference is indistinguishable from a hand edit, and this tool
does not get to guess which it is.`,
	RunE: runApply,
	Args: cobra.NoArgs,
}

func init() {
	rootCmd.AddCommand(applyCmd)
	applyCmd.Flags().StringVar(&applyCorpus, "corpus", "", "path to a fragment corpus JSON file (required)")
	applyCmd.Flags().StringVar(&applyTarget, "target", "", "target name to look up in --targets")
	applyCmd.Flags().StringVar(&applyTargets, "targets", "", "path to a targets JSON file (array of {name, host, profile, harness})")
	applyCmd.Flags().StringVar(&applyHost, "host", "", "machine id to compile for (ad-hoc target, with --profile and --harness)")
	applyCmd.Flags().StringVar(&applyProfile, "profile", "", "agent id to compile for (ad-hoc target)")
	applyCmd.Flags().StringVar(&applyHarness, "harness", "", "harness to compile for: openclaw, claude, hermes, or codex (ad-hoc target)")
	applyCmd.Flags().StringVar(&applyRoot, "root", "", "directory to write into (required)")
	applyCmd.Flags().BoolVar(&applyCommit, "commit", false, "actually write; without it this is a dry run")
	applyCmd.Flags().BoolVar(&applyForce, "force", false, "permit overwriting files whose content differs")
	applyCmd.Flags().BoolVar(&applyNoBackup, "no-backup", false, "skip backups (safe only when --root is a clean git tree)")
	applyCmd.Flags().StringVar(&applyBackupTo, "backup-to", "", "backup directory; defaults to <root>/.soul-forge-backup/<timestamp>")
	applyCmd.Flags().BoolVar(&applyJSON, "json", false, "emit the plan as JSON")
	applyCmd.MarkFlagRequired("corpus")
	applyCmd.MarkFlagRequired("root")
}

func runApply(cmd *cobra.Command, args []string) error {
	corpus, err := loadCorpus(applyCorpus)
	if err != nil {
		return err
	}
	target, err := resolveTarget()
	if err != nil {
		return err
	}

	backupDir := ""
	if !applyNoBackup {
		backupDir = applyBackupTo
		if backupDir == "" {
			backupDir = filepath.Join(applyRoot, ".soul-forge-backup", time.Now().UTC().Format("20060102-150405"))
		}
	}

	plan, err := compile.Plan(corpus, target, applyRoot, backupDir)
	if err != nil {
		return err
	}

	if applyJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(plan); err != nil {
			return err
		}
	} else {
		printPlan(plan)
	}

	if !applyCommit {
		fmt.Fprintln(os.Stderr, "\ndry run — nothing written. Re-run with --commit to apply.")
		return nil
	}

	if plan.Destructive() && !applyForce {
		return fmt.Errorf("plan overwrites %d file(s) whose content differs from the compile output.\n"+
			"That difference may be a hand edit this corpus has never seen. Re-run with --force to replace them",
			countDestructive(plan))
	}

	res, err := plan.Commit()
	if err != nil {
		return err
	}

	for _, p := range res.Written {
		fmt.Printf("wrote %s\n", p)
	}
	if len(res.BackedUp) > 0 {
		fmt.Printf("\nbacked up %d overwritten file(s) to %s\n", len(res.BackedUp), res.BackupDir)
	}
	if len(res.Written) == 0 {
		fmt.Println("nothing to do — disk already matches the corpus")
	}
	return nil
}

func printPlan(p compile.ApplyPlan) {
	fmt.Printf("target %s -> %s\n\n", p.Target, p.Root)
	changes := p.Changes()
	if len(changes) == 0 {
		fmt.Println("no changes — disk already matches the corpus")
	}
	for _, f := range changes {
		mark := " "
		note := f.Reason
		if f.Destructive() {
			mark = "!"
			// The size change, always, on every overwrite. --force fires the same way
			// for a one-line correction and for a corpus gap that guts a rulebook; the
			// operator can only tell them apart if the numbers are on screen.
			before, after := f.Delta()
			note = fmt.Sprintf("%s (%d -> %d bytes, %+d%%)", f.Reason, before, after, pctChange(before, after))
		}
		fmt.Printf("%s %-10s %-14s %s\n", mark, f.Action, f.Path, note)
	}
	// Declined paths are printed too. A plan that lists only what it will do reads as
	// a complete account of what it considered, and it is not — MEMORY.md being absent
	// from the list should never be something the operator has to infer.
	for _, f := range p.Files {
		if f.Action == compile.ActionNone {
			fmt.Printf("  %-10s %-14s %s\n", "skip", f.Path, f.Reason)
		}
	}
}

// pctChange reports the size change as a percentage. A file growing from nothing is
// reported as +100 rather than a division by zero — but that case is ActionCreate, not
// an overwrite, so it never reaches here in practice.
func pctChange(before, after int) int {
	if before == 0 {
		return 100
	}
	return (after - before) * 100 / before
}

func countDestructive(p compile.ApplyPlan) int {
	n := 0
	for _, f := range p.Files {
		if f.Destructive() {
			n++
		}
	}
	return n
}

// loadCorpus reads a fragment corpus and validates every fragment before anything is
// compiled from it. An invalid fragment reaching Compile would fail an invariant with
// a message about scope; failing here says the file is malformed, which is the truth.
//
// Two formats are accepted: a flat fragment array (what onboard and review emit) and
// a profile object with an extends chain (what clone emits). A loader accepting only
// one would orphan the other command's output.
func loadCorpus(path string) ([]fragment.Fragment, error) {
	frags, _, err := loadCorpusWithOverrides(path)
	return frags, err
}

// loadCorpusWithOverrides is loadCorpus plus the override records a profile chain
// produced. Only audit consumes the overrides — a downstream box changing inherited
// doctrine is worth a report, not a failure.
func loadCorpusWithOverrides(path string) ([]fragment.Fragment, []fragment.Override, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("corpus: %w", err)
	}
	if fragment.IsProfileFile(b) {
		c, err := fragment.LoadProfile(path)
		if err != nil {
			return nil, nil, err
		}
		frags, overrides, err := c.Resolve() // Resolve validates every fragment
		if err != nil {
			return nil, nil, fmt.Errorf("corpus %s: %w", path, err)
		}
		return frags, overrides, nil
	}
	var frags []fragment.Fragment
	if err := json.Unmarshal(b, &frags); err != nil {
		return nil, nil, fmt.Errorf("corpus %s: %w", path, err)
	}
	for i, f := range frags {
		if err := f.Validate(); err != nil {
			return nil, nil, fmt.Errorf("corpus %s: fragment %d (%s): %w", path, i, f.ID, err)
		}
	}
	return frags, nil, nil
}

// targetDef is one entry in a --targets file. Targets are user data, not tool
// constants: an earlier version baked its author's agent and machine into the
// binary here, which is exactly the hardcoding this tool exists to kill.
type targetDef struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Profile string `json:"profile"`
	Harness string `json:"harness"`
}

// resolveTarget builds apply's compile target from its flags.
func resolveTarget() (compile.Target, error) {
	return resolveTargetFrom(applyTarget, applyTargets, applyHost, applyProfile, applyHarness)
}

// resolveTargetFrom builds a compile target from either an ad-hoc
// (host, profile, harness) triple or a name looked up in a targets file.
// Shared by every command that compiles.
func resolveTargetFrom(name, targetsFile, host, profile, harness string) (compile.Target, error) {
	adHoc := host != "" || profile != "" || harness != ""
	named := name != "" || targetsFile != ""

	switch {
	case adHoc && named:
		return compile.Target{}, fmt.Errorf("use either --host/--profile/--harness or --targets/--target, not both")
	case adHoc:
		if host == "" || profile == "" || harness == "" {
			return compile.Target{}, fmt.Errorf("an ad-hoc target needs all three of --host, --profile, --harness")
		}
		return compile.Target{
			Name:     fmt.Sprintf("%s/%s/%s", host, profile, harness),
			Selector: fragment.Selector{Host: host, Profile: profile, Harness: harness},
		}, nil
	case name != "" && targetsFile != "":
		defs, err := loadTargets(targetsFile)
		if err != nil {
			return compile.Target{}, err
		}
		var known []string
		for _, d := range defs {
			if d.Name == name {
				return compile.Target{
					Name:     d.Name,
					Selector: fragment.Selector{Host: d.Host, Profile: d.Profile, Harness: d.Harness},
				}, nil
			}
			known = append(known, d.Name)
		}
		// Unknown names are an error listing the known ones. Defaulting to the first
		// entry would write a typo'd target's output into a real workspace.
		return compile.Target{}, fmt.Errorf("target %q not in %s (known: %s)", name, targetsFile, strings.Join(known, ", "))
	default:
		return compile.Target{}, fmt.Errorf("no target: pass --host/--profile/--harness, or --targets <file> with --target <name>")
	}
}

func loadTargets(path string) ([]targetDef, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("targets: %w", err)
	}
	var defs []targetDef
	if err := json.Unmarshal(b, &defs); err != nil {
		return nil, fmt.Errorf("targets %s: %w", path, err)
	}
	for i, d := range defs {
		if d.Name == "" || d.Host == "" || d.Profile == "" || d.Harness == "" {
			return nil, fmt.Errorf("targets %s: entry %d: name, host, profile, and harness are all required", path, i)
		}
	}
	return defs, nil
}
