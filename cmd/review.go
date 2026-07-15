package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cyperx84/soul-forge/internal/ingest"
	"github.com/spf13/cobra"
)

// review is the judgment step, batched.
//
// It emits a questionnaire and consumes the answers. Two commands rather than a
// prompt loop, because the reviewer is usually a harness driving the CLI, and because
// a written answer file is reviewable, diffable, and re-runnable — a terminal prompt
// session is none of those.
//
// The measured collapse on the real corpus is 7.6x: 280 per-line per-axis decisions
// become 37 batched ones. That ratio is the difference between an afternoon and an
// abandoned tool.

var (
	reviewHost     string
	reviewAgents   []string
	reviewAnswers  string
	reviewOutput   string
	reviewShowLine bool
)

var reviewCmd = &cobra.Command{
	Use:   "review <path...>",
	Short: "Batch the tagging decisions ingest could not resolve",
	Long: `Groups ingest's unresolved lines into one question per (file, section, axis set)
and writes them as a questionnaire.

Without --answers it emits the questions. With --answers it reads the filled-in
file and emits the confirmed fragment corpus.

Markdown sections do the grouping because a heading is the author's own statement
that these lines belong together. One answer tags every line under it — which is
the point, and also the risk, so every fragment records the batch that tagged it.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runReview,
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	reviewCmd.Flags().StringVar(&reviewHost, "host", "", "machine id these files came from (e.g. m4-mini)")
	reviewCmd.Flags().StringSliceVar(&reviewAgents, "agents", nil, "known agent ids, so lines naming one get flagged")
	reviewCmd.Flags().StringVar(&reviewAnswers, "answers", "", "path to a filled-in answers file; emits fragments instead of questions")
	reviewCmd.Flags().StringVar(&reviewOutput, "out", "", "write output here instead of stdout")
	reviewCmd.Flags().BoolVar(&reviewShowLine, "lines", false, "show every member line of each batch, not just the first two")
}

// Question is one batch rendered for a reviewer: what is being asked, why it could
// not be answered mechanically, and which lines the answer will tag.
type Question struct {
	Key     string   `json:"key"`
	File    string   `json:"file"`
	Section string   `json:"section"`
	Axes    []string `json:"axes"`

	// Reasons is why each axis is unresolved. A reviewer answering without these is
	// guessing on the tool's behalf, which defeats the point of asking.
	Reasons map[string][]string `json:"reasons"`

	// Suggestion is ingest's proposed value per axis — the placeholder it refused to
	// treat as decided. Offered so the reviewer approves rather than authors, and
	// labelled a suggestion so approving stays a decision.
	Suggestion map[string]string `json:"suggestion"`

	// Lines are the members this answer will tag.
	Lines []QuestionLine `json:"lines"`

	// Answer is filled in by the reviewer: axis name to value.
	Answer map[string]string `json:"answer"`

	// Skip drops the batch: its lines produce no fragments.
	Skip bool `json:"skip,omitempty"`
}

// QuestionLine is one member line with its origin and any flags.
type QuestionLine struct {
	Origin string   `json:"origin"`
	Text   string   `json:"text"`
	Flags  []string `json:"flags,omitempty"`
}

func runReview(cmd *cobra.Command, args []string) error {
	files, err := expand(args)
	if err != nil {
		return err
	}
	proposals, err := proposeAll(files, ingest.Options{Host: reviewHost, Agents: reviewAgents})
	if err != nil {
		return err
	}
	batches := ingest.Batches(proposals)

	if reviewAnswers == "" {
		return emitQuestions(cmd, proposals, batches)
	}
	return applyAnswers(cmd, batches)
}

func proposeAll(files []string, opts ingest.Options) ([]ingest.Proposal, error) {
	var out []ingest.Proposal
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		cands, err := ingest.Extract(path, f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("extract %s: %w", path, err)
		}
		for _, c := range cands {
			out = append(out, ingest.Propose(c, opts))
		}
	}
	return out, nil
}

func emitQuestions(cmd *cobra.Command, ps []ingest.Proposal, batches []ingest.Batch) error {
	bill := ingest.Measure(ps)
	qs := make([]Question, len(batches))
	for i, b := range batches {
		q := Question{
			Key: b.Key(), File: b.Path, Section: b.Section, Axes: b.Axes,
			Reasons:    b.Reasons(),
			Suggestion: map[string]string{},
			Answer:     map[string]string{},
		}
		for _, axis := range b.Axes {
			q.Suggestion[axis] = suggestion(b, axis)
			q.Answer[axis] = "" // reviewer fills this
		}
		for _, m := range b.Members {
			q.Lines = append(q.Lines, QuestionLine{
				Origin: m.Candidate.Origin(), Text: m.Candidate.Text, Flags: m.Flags,
			})
		}
		qs[i] = q
	}

	w := cmd.OutOrStdout()
	if reviewOutput != "" {
		f, err := os.Create(reviewOutput)
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
			"lines":                       bill.Lines,
			"resolved_by_signal":          bill.Resolved,
			"decisions_per_axis":          bill.PerAxis,
			"decisions_batched":           bill.Batched,
			"collapse":                    fmt.Sprintf("%.1fx", bill.Collapse()),
			"how_to_fill":                 "set answer.<axis> for each question, or set skip:true to drop the batch. Every question must be answered or skipped.",
			"suggestion_is_not_an_answer": "suggestion is what ingest proposed and refused to treat as decided; copying it is a decision, not a default",
		},
		"questions": qs,
	}); err != nil {
		return err
	}

	if reviewOutput != "" {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"%d questions covering %d lines (%.1fx collapse from %d per-axis decisions) → %s\n",
			bill.Batched, bill.Lines-bill.Resolved, bill.Collapse(), bill.PerAxis, reviewOutput)
	}
	return nil
}

// suggestion returns the proposed value for an axis when every member agrees, or ""
// when they differ. A single suggestion standing in for members that disagree would
// be the tool deciding by majority — which is a guess wearing a helpful hat.
func suggestion(b ingest.Batch, axis string) string {
	seen := map[string]bool{}
	for _, m := range b.Members {
		seen[proposedValue(m, axis)] = true
	}
	if len(seen) != 1 {
		return ""
	}
	for v := range seen {
		return v
	}
	return ""
}

func proposedValue(p ingest.Proposal, axis string) string {
	switch axis {
	case "host":
		return p.Host.Value
	case "profile":
		return p.Profile.Value
	case "harness":
		return p.Harness.Value
	case "lifecycle":
		return p.Lifecycle.Value
	case "kind":
		return p.Kind.Value
	}
	return ""
}

func applyAnswers(cmd *cobra.Command, batches []ingest.Batch) error {
	raw, err := os.ReadFile(reviewAnswers)
	if err != nil {
		return fmt.Errorf("read answers: %w", err)
	}
	var doc struct {
		Questions []Question `json:"questions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse answers: %w", err)
	}

	answers := make([]ingest.Answer, 0, len(doc.Questions))
	var blank []string
	for _, q := range doc.Questions {
		if q.Skip {
			answers = append(answers, ingest.Answer{Key: q.Key, Skip: true})
			continue
		}
		// An empty string is an unfilled field, not a decision. Passing it through
		// would fail Validate downstream with a confusing message; naming it here
		// tells the reviewer exactly which question they left blank.
		for _, axis := range q.Axes {
			if strings.TrimSpace(q.Answer[axis]) == "" {
				blank = append(blank, q.Key+" ("+axis+")")
			}
		}
		answers = append(answers, ingest.Answer{Key: q.Key, Values: q.Answer})
	}
	if len(blank) > 0 {
		sort.Strings(blank)
		return fmt.Errorf("unfilled answers — fill them or set skip:true:\n  %s", strings.Join(blank, "\n  "))
	}

	res, err := ingest.Apply(batches, answers, idFromBatch)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if reviewOutput != "" {
		f, err := os.Create(reviewOutput)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{
		"fragments": res.Fragments,
		"applied":   res.Applied,
		"skipped":   skippedKeys(res.Skipped),
	}); err != nil {
		return err
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%d fragments from %d answered batches, %d skipped\n",
		len(res.Fragments), len(batches)-len(res.Skipped), len(res.Skipped))
	return nil
}

func skippedKeys(bs []ingest.Batch) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Key()
	}
	return out
}

// idFromBatch names a fragment from its section and position.
//
// This is a placeholder and it is the weakest thing in the review flow: an ID is a
// name, naming is a human's job, and "how-we-operate-2" encodes the source file's
// layout into the corpus — the file-as-owner bug creeping back through the ID field.
// It is here so the pipeline runs end to end; the interview step (7) is where a
// reviewer names fragments properly.
func idFromBatch(p ingest.Proposal, b ingest.Batch, i int) string {
	slug := strings.ToLower(b.Section)
	slug = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r == ' ', r == '-', r == '_':
			return '-'
		}
		return -1
	}, slug)
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "fragment"
	}
	return fmt.Sprintf("%s-%d", slug, i+1)
}
