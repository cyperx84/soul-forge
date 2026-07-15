package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cyperx84/soul-forge/internal/fragment"
	"github.com/cyperx84/soul-forge/internal/ingest"
	"github.com/spf13/cobra"
)

// merge is where the hand-sync dies, and it runs before review.
//
// Ordering is not taste. Merging first means a collapsed group is one question in
// review instead of two, and — the part that matters — `apply` must never run before
// merge, or it writes today's duplication back to disk under a compiler's authority,
// laundering hand-sync as compiled output.
//
// Same two-command shape as review, for the same reasons: the reviewer is usually a
// harness driving the CLI, and a written answer file is reviewable, diffable, and
// re-runnable.

var (
	mergeHost    string
	mergeAgents  []string
	mergeAnswers string
	mergeOutput  string
	mergeFloor   float64
	mergeTop     int
)

var mergeCmd = &cobra.Command{
	Use:   "merge <path...>",
	Short: "Decide which duplicated lines are one fragment",
	Long: `Ranks near-identical lines across the ingested files and asks, per pair,
whether they are one fragment or two.

Without --answers it emits the questions. With --answers it reads the filled-in
file and emits the merged proposal set.

A merge is a widening, not an equality claim. Two lines under openclaw and claude
are evidence for those two harnesses; the axis model cannot say "these two and no
others", so merging them says any — which sends the line to every harness. Each
question names the targets the merge would newly reach. That is the decision being
made, and it is bigger than "are these the same".

Unanswered questions are declined, leaving both lines. That default is safe (it is
today's state on disk) and it is reported, because a merge step nobody answered
looks exactly like one that never ran.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMerge,
}

func init() {
	rootCmd.AddCommand(mergeCmd)
	mergeCmd.Flags().StringVar(&mergeHost, "host", "", "machine id these files came from (e.g. m4-mini)")
	mergeCmd.Flags().StringSliceVar(&mergeAgents, "agents", nil, "known agent ids")
	mergeCmd.Flags().StringVar(&mergeAnswers, "answers", "", "path to a filled-in answers file; emits merged proposals instead of questions")
	mergeCmd.Flags().StringVar(&mergeOutput, "out", "", "write output here instead of stdout")
	mergeCmd.Flags().Float64Var(&mergeFloor, "floor", ingest.FloorDefault,
		"drop pairs below this score. Bounds list length only — it is not a similarity judgment, and the score is corpus-relative (see ingest.Duplicates)")
	mergeCmd.Flags().IntVar(&mergeTop, "top", 40,
		"ask only the top N ranked pairs. Rank is the contract; the rest are reported as unasked, never silently dropped")
}

// MergeQ is one merge question rendered for a reviewer.
type MergeQ struct {
	Key   string  `json:"key"`
	Score float64 `json:"score"`

	// A and B are the two lines, with their origins. Both are shown in full: the
	// decision is about the text, and a truncated rule is a rule nobody read.
	A MergeSide `json:"a"`
	B MergeSide `json:"b"`

	// Shared are the rare terms that surfaced the pair — why it is being asked.
	Shared []string `json:"shared_terms,omitempty"`

	// Widens maps each axis the merge would widen to `any`, to the concrete values
	// being given up.
	Widens map[string]string `json:"widens,omitempty"`

	// Blast names the targets the merged fragment would reach that neither line
	// reaches today. This is the cost of answering yes.
	Blast []string `json:"newly_reaches,omitempty"`

	// NeedsText and NeedsKind are decisions the merge cannot make.
	NeedsText bool `json:"needs_text,omitempty"`
	NeedsKind bool `json:"needs_kind,omitempty"`

	// Refused explains why this pair cannot be merged at all.
	Refused string `json:"refused,omitempty"`

	// Merge, Text, and Kind are filled in by the reviewer.
	Merge bool   `json:"merge"`
	Text  string `json:"text,omitempty"`
	Kind  string `json:"kind,omitempty"`
}

// MergeSide is one line of a pair.
type MergeSide struct {
	Origin string   `json:"origin"`
	Text   string   `json:"text"`
	Flags  []string `json:"flags,omitempty"`
}

func runMerge(cmd *cobra.Command, args []string) error {
	files, err := expand(args)
	if err != nil {
		return err
	}
	opts := ingest.Options{Host: mergeHost, Agents: mergeAgents}
	proposals, err := proposeAll(files, opts)
	if err != nil {
		return err
	}

	cands := make([]ingest.Candidate, len(proposals))
	for i, p := range proposals {
		cands[i] = p.Candidate
	}
	pairs := ingest.Duplicates(cands, mergeFloor)
	questions := ingest.MergeQuestions(pairs, proposals, mergeTargets(mergeHost, mergeAgents))

	if mergeAnswers == "" {
		return emitMergeQuestions(cmd, questions)
	}
	return applyMergeAnswers(cmd, proposals, questions)
}

// mergeTargets is the fleet the widening is measured against.
//
// These are the harnesses that exist. A merged fragment tagged harness:any reaches all
// of them, and a reviewer who is not shown that is answering a question they were not
// asked. Host and profile are held at the ingested values because a merge across those
// axes is measured the same way — what does saying `any` here newly reach.
func mergeTargets(host string, agents []string) []ingest.MergeTarget {
	if host == "" {
		host = fragment.AxisAny
	}
	profile := fragment.AxisAny
	if len(agents) > 0 {
		profile = agents[0]
	}
	var out []ingest.MergeTarget
	for _, h := range []string{
		fragment.HarnessOpenClaw, fragment.HarnessClaude,
		fragment.HarnessHermes, fragment.HarnessCodex,
	} {
		out = append(out, ingest.MergeTarget{
			Name:     h,
			Selector: fragment.Selector{Host: host, Profile: profile, Harness: h},
		})
	}
	return out
}

func emitMergeQuestions(cmd *cobra.Command, questions []ingest.MergeQuestion) error {
	asked := questions
	unasked := 0
	if mergeTop > 0 && len(questions) > mergeTop {
		asked = questions[:mergeTop]
		unasked = len(questions) - mergeTop
	}

	qs := make([]MergeQ, len(asked))
	var refused, widening int
	for i, q := range asked {
		qs[i] = MergeQ{
			Key: q.Key, Score: q.Pair.Score,
			A:         MergeSide{Origin: q.Pair.A.Origin(), Text: q.Pair.A.Text},
			B:         MergeSide{Origin: q.Pair.B.Origin(), Text: q.Pair.B.Text},
			Shared:    q.Pair.Shared,
			Widens:    q.Widens,
			Blast:     q.Blast,
			NeedsText: q.NeedsText,
			NeedsKind: q.NeedsKind,
			Refused:   q.Refused,
		}
		if q.Refused != "" {
			refused++
		}
		if len(q.Widens) > 0 {
			widening++
		}
	}

	w := cmd.OutOrStdout()
	if mergeOutput != "" {
		f, err := os.Create(mergeOutput)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{
		"bill": map[string]any{
			"pairs_ranked":           len(questions),
			"asked":                  len(asked),
			"unasked":                unasked,
			"refused":                refused,
			"would_widen":            widening,
			"how_to_fill":            "set merge:true on pairs that are one fragment. Supply text when needs_text, kind when needs_kind. Leaving a pair alone declines it.",
			"rank_is_the_contract":   "read top-down and stop when the pairs stop being real. score is corpus-relative and cannot be compared across runs",
			"a_merge_is_a_widening":  "newly_reaches names targets the merged line would reach that neither line reaches today. Some rules are false there",
			"unanswered_is_declined": "declining leaves both lines, which is today's state on disk — safe, and it means no dedup happened",
		},
		"questions": qs,
	}); err != nil {
		return err
	}

	// Report the cap. A tool that quietly asks about 40 of 889 pairs, then says
	// "merge complete", has told you it covered something it never looked at.
	if unasked > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"asking the top %d of %d ranked pairs; %d not asked (raise --top to see them — they stay duplicated either way)\n",
			len(asked), len(questions), unasked)
	}
	if mergeOutput != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "%d merge questions (%d would widen an axis, %d refused) → %s\n",
			len(asked), widening, refused, mergeOutput)
	}
	return nil
}

func applyMergeAnswers(cmd *cobra.Command, ps []ingest.Proposal, questions []ingest.MergeQuestion) error {
	raw, err := os.ReadFile(mergeAnswers)
	if err != nil {
		return fmt.Errorf("read answers: %w", err)
	}
	var doc struct {
		Questions []MergeQ `json:"questions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse answers: %w", err)
	}

	answers := make([]ingest.MergeAnswer, 0, len(doc.Questions))
	for _, q := range doc.Questions {
		if !q.Merge {
			continue
		}
		answers = append(answers, ingest.MergeAnswer{Key: q.Key, Merge: true, Text: q.Text, Kind: q.Kind})
	}

	res, err := ingest.Merge(ps, questions, answers)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if mergeOutput != "" {
		f, err := os.Create(mergeOutput)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{
		"proposals": mergedView(res.Proposals),
		"merged":    res.Merged,
		"declined":  len(res.Declined),
		"refused":   len(res.Refused),
	}); err != nil {
		return err
	}

	fmt.Fprintf(cmd.ErrOrStderr(),
		"%d lines → %d proposals (%d collapsed); %d pairs left duplicated, %d unmergeable\n",
		len(ps), len(res.Proposals), len(ps)-len(res.Proposals), len(res.Declined), len(res.Refused))
	return nil
}

// mergedView renders proposals with their provenance, which is the reviewable part: a
// collapse that named one origin would hide the hand-sync it just killed.
func mergedView(ps []ingest.Proposal) []map[string]any {
	out := make([]map[string]any, len(ps))
	for i, p := range ps {
		m := map[string]any{
			"text":    p.Line(),
			"origins": p.Origins(),
			"host":    p.Host.Value,
			"profile": p.Profile.Value,
			"harness": p.Harness.Value,
			"kind":    p.Kind.Value,
		}
		if u := p.Unresolved(); len(u) > 0 {
			m["unresolved"] = u
		}
		if len(p.Flags) > 0 {
			m["flags"] = p.Flags
		}
		out[i] = m
	}
	return out
}
