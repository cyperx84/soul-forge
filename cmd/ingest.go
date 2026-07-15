package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cyperx84/soul-forge/internal/ingest"
	"github.com/spf13/cobra"
)

// ingest is the migration path: it reverse-compiles the guidance files a machine
// already has into proposed fragments, and reports which lines say the same thing in
// more than one place.
//
// It writes nothing. The output is a review artifact, because tagging a line along
// four scope axes is the one step in this pipeline that needs judgment — and a wrong
// tag is the bug the whole fragment model exists to prevent. Proposals become
// fragments through ingest.Proposal.Confirm and nowhere else.

var (
	ingestHost   string
	ingestAgents []string
	ingestFloor  float64
	ingestJSON   bool
	ingestTopN   int
)

var ingestCmd = &cobra.Command{
	Use:   "ingest <path...>",
	Short: "Reverse-compile existing agent files into proposed fragments",
	Long: `Reads existing agent guidance files and proposes a fragment per authored line,
tagged along the four scope axes, plus a ranked report of lines that duplicate
each other across or within files.

ingest never writes and never decides. Axes it cannot determine from a file's
documented role are reported unresolved, for a human or harness to confirm.

Duplicate pairs are ranked, not thresholded: the similarity score is computed
from the ingested set, so it is comparable within one run and meaningless across
runs. Read the list top-down and stop when the pairs stop being real.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runIngest,
}

func init() {
	rootCmd.AddCommand(ingestCmd)
	ingestCmd.Flags().StringVar(&ingestHost, "host", "", "machine id these files came from (e.g. m4-mini)")
	ingestCmd.Flags().StringSliceVar(&ingestAgents, "agents", nil, "known agent ids, so lines naming one get flagged")
	ingestCmd.Flags().Float64Var(&ingestFloor, "floor", ingest.FloorDefault, "drop duplicate pairs below this score (bounds list length; not a similarity judgment)")
	ingestCmd.Flags().IntVar(&ingestTopN, "top", 20, "show at most this many duplicate pairs (0 = all)")
	ingestCmd.Flags().BoolVar(&ingestJSON, "json", false, "emit proposals and pairs as JSON")
}

func runIngest(cmd *cobra.Command, args []string) error {
	files, err := expand(args)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no markdown files found in %s", strings.Join(args, ", "))
	}

	var cands []ingest.Candidate
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			// An unreadable file is an error, never silently zero candidates: a
			// missing rule looks identical to a rule that was never there.
			return fmt.Errorf("read %s: %w", path, err)
		}
		got, err := ingest.Extract(path, f)
		f.Close()
		if err != nil {
			return fmt.Errorf("extract %s: %w", path, err)
		}
		cands = append(cands, got...)
	}

	opts := ingest.Options{Host: ingestHost, Agents: ingestAgents}
	proposals := make([]ingest.Proposal, len(cands))
	for i, c := range cands {
		proposals[i] = ingest.Propose(c, opts)
	}
	pairs := ingest.Duplicates(cands, ingestFloor)

	if ingestJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"proposals": proposals,
			"pairs":     pairs,
		})
	}
	report(cmd.OutOrStdout(), files, proposals, pairs)
	return nil
}

// expand turns directories into their markdown files and passes files through.
func expand(args []string) ([]string, error) {
	var out []string
	for _, a := range args {
		info, err := os.Stat(a)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, a)
			continue
		}
		entries, err := os.ReadDir(a)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			out = append(out, filepath.Join(a, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func report(w interface{ Write([]byte) (int, error) }, files []string, ps []ingest.Proposal, pairs []ingest.Pair) {
	p := func(format string, a ...any) { fmt.Fprintf(w, format, a...) }

	p("ingested %d files, %d candidate lines\n\n", len(files), len(ps))

	// Unresolved axes are the reviewer's actual bill. Show it up front: it is the
	// number that decides whether this migration gets finished or abandoned.
	byAxis := map[string]int{}
	fullyResolved := 0
	for _, prop := range ps {
		u := prop.Unresolved()
		if len(u) == 0 {
			fullyResolved++
		}
		for _, a := range u {
			byAxis[a]++
		}
	}
	p("REVIEW BILL\n")
	p("  %d of %d lines fully resolved by signal (no judgment needed)\n", fullyResolved, len(ps))
	axes := make([]string, 0, len(byAxis))
	for a := range byAxis {
		axes = append(axes, a)
	}
	sort.Strings(axes)
	for _, a := range axes {
		p("  %-9s unresolved on %d lines\n", a, byAxis[a])
	}

	var flagged []ingest.Proposal
	for _, prop := range ps {
		if len(prop.Flags) > 0 {
			flagged = append(flagged, prop)
		}
	}
	if len(flagged) > 0 {
		p("\nFLAGS (%d)\n", len(flagged))
		for _, prop := range flagged {
			p("  %s\n    %s\n", prop.Candidate.Origin(), truncate(prop.Candidate.Text, 90))
			for _, f := range prop.Flags {
				p("    ! %s\n", f)
			}
		}
	}

	shown := pairs
	if ingestTopN > 0 && len(shown) > ingestTopN {
		shown = shown[:ingestTopN]
	}
	p("\nDUPLICATE CANDIDATES (%d shown of %d; ranked — read top-down, stop when they stop being real)\n", len(shown), len(pairs))
	for i, pr := range shown {
		scope := "cross-file"
		if pr.SameFile {
			scope = "same-file"
		}
		p("\n  #%d  %.3f  %s  [%s]\n", i+1, pr.Score, strings.Join(pr.Shared, " "), scope)
		p("      %s  %s\n", pr.A.Origin(), truncate(pr.A.Text, 84))
		p("      %s  %s\n", pr.B.Origin(), truncate(pr.B.Text, 84))
	}
	if ingestTopN > 0 && len(pairs) > ingestTopN {
		// Never let a cap look like coverage.
		p("\n  %d further pairs below the cut — re-run with --top 0 to see them all\n", len(pairs)-ingestTopN)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
