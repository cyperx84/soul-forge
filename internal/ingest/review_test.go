package ingest

import (
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

// realProposals ingests lines as they appear in the actual workspace files, sections
// and all. The batching claim is about how Chris really writes markdown, so testing
// it against invented sections would only prove the grouper agrees with itself.
func realProposals() []Proposal {
	cands := []Candidate{
		// AGENTS.md "How we operate": one profile answer for the whole section.
		{Text: "Act first, explain after. Execute, then report.", Path: "/Users/c/.openclaw/workspace/AGENTS.md", Line: 30, Section: "How we operate"},
		{Text: "Confirm only for: destructive actions, anything leaving the machine.", Path: "/Users/c/.openclaw/workspace/AGENTS.md", Line: 31, Section: "How we operate"},
		{Text: "Manual first; automate once it's understood.", Path: "/Users/c/.openclaw/workspace/AGENTS.md", Line: 35, Section: "How we operate"},
		// AGENTS.md "Red lines": same axis set, different section — a separate question.
		{Text: "Never exfiltrate private data.", Path: "/Users/c/.openclaw/workspace/AGENTS.md", Line: 44, Section: "Red lines"},
		{Text: "`trash` > `rm`.", Path: "/Users/c/.openclaw/workspace/AGENTS.md", Line: 46, Section: "Red lines"},
		// USER.md: fully resolved by signal — not a question, must not enter the queue.
		{Text: "Casual, swears freely, low fluff.", Path: "/Users/c/.openclaw/workspace/USER.md", Line: 18, Section: "How he thinks"},
	}
	out := make([]Proposal, len(cands))
	for i, c := range cands {
		out[i] = Propose(c, Options{Host: "m4-mini", Agents: []string{"klaw"}})
	}
	return out
}

func TestBatchesGroupBySectionNotFile(t *testing.T) {
	bs := Batches(realProposals())

	// Two AGENTS.md sections share an axis set but are different questions: "How we
	// operate" is fleet-wide doctrine, "Red lines" might be too — but that is the
	// reviewer's call to make twice, not the grouper's to merge into one.
	var sections []string
	for _, b := range bs {
		if strings.HasSuffix(b.Path, "AGENTS.md") {
			sections = append(sections, b.Section)
		}
	}
	if len(sections) != 2 {
		t.Fatalf("AGENTS.md produced %d batches (%v), want 2 — one per section", len(sections), sections)
	}

	for _, b := range bs {
		if b.Section == "How we operate" && len(b.Members) != 3 {
			t.Errorf("'How we operate' has %d members, want 3 — the section's lines should be one question", len(b.Members))
		}
	}
}

// A queue padded with items needing no answer trains a reviewer to skim, and skimming
// is how the one question that mattered gets waved through.
func TestBatchesExcludeFullyResolvedLines(t *testing.T) {
	for _, b := range Batches(realProposals()) {
		for _, m := range b.Members {
			if len(m.Unresolved()) == 0 {
				t.Errorf("%s entered the queue with nothing unresolved", m.Candidate.Origin())
			}
		}
	}
}

// The claim that justifies this whole file: grouping collapses the bill. If sections
// ever stop grouping like lines, this fails rather than quietly returning 1.0x while
// the report still prints a saving.
func TestMeasureShowsRealCollapse(t *testing.T) {
	bill := Measure(realProposals())

	if bill.PerAxis <= bill.Batched {
		t.Fatalf("per-axis bill %d, batched bill %d — batching bought nothing; the grouping key is wrong",
			bill.PerAxis, bill.Batched)
	}
	if got := bill.Collapse(); got < 2.0 {
		t.Errorf("collapse = %.1fx, want at least 2x — below that, batching is not worth the wrong-answer blast radius", got)
	}
	if bill.Resolved == 0 {
		t.Error("no lines resolved by signal — every line becoming a question means the signals are dead")
	}
}

func TestMeasureCollapseHandlesEmptyCorpus(t *testing.T) {
	if got := Measure(nil).Collapse(); got != 1 {
		t.Errorf("Collapse on an empty corpus = %v, want 1 (no division by zero, no invented saving)", got)
	}
}

func TestBatchesAreDeterministic(t *testing.T) {
	a, b := Batches(realProposals()), Batches(realProposals())
	if len(a) != len(b) {
		t.Fatalf("run 1 gave %d batches, run 2 gave %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Key() != b[i].Key() {
			t.Fatalf("order differs at %d: %s vs %s", i, a[i].Key(), b[i].Key())
		}
	}
}

func answerAll(bs []Batch, values map[string]string) []Answer {
	out := make([]Answer, len(bs))
	for i, b := range bs {
		v := map[string]string{}
		for _, axis := range b.Axes {
			v[axis] = values[axis]
		}
		out[i] = Answer{Key: b.Key(), Values: v}
	}
	return out
}

func TestApplyAnswersEveryMemberOfABatch(t *testing.T) {
	bs := Batches(realProposals())
	res, err := Apply(bs, answerAll(bs, map[string]string{
		"host": fragment.AxisAny, "profile": fragment.AxisAny,
		"harness": fragment.AxisAny, "kind": fragment.KindRule,
	}), func(p Proposal, b Batch, i int) string {
		return strings.ToLower(strings.ReplaceAll(b.Section, " ", "-")) + "-" + itoa(i)
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	var members int
	for _, b := range bs {
		members += len(b.Members)
	}
	if len(res.Fragments) != members {
		t.Fatalf("got %d fragments from %d batched members — a batch answer must reach every member",
			len(res.Fragments), members)
	}
	// Every fragment traces back to the answer that tagged it, so a wrong batch
	// answer is reversible instead of hunted line by line.
	if len(res.Applied) != len(res.Fragments) {
		t.Errorf("%d fragments but %d traces — untraceable tags cannot be reversed", len(res.Fragments), len(res.Applied))
	}
	for _, f := range res.Fragments {
		if f.Source == "" {
			t.Errorf("fragment %q lost its provenance", f.ID)
		}
	}
}

// "I didn't get to it" and "I decided to drop it" are different states. Silently
// treating the first as the second loses rules without saying so — the same class as
// blurring changed with verified.
func TestApplyRejectsUnansweredBatch(t *testing.T) {
	bs := Batches(realProposals())
	if len(bs) < 2 {
		t.Fatal("precondition: need at least 2 batches")
	}
	partial := answerAll(bs[:1], map[string]string{
		"host": fragment.AxisAny, "profile": fragment.AxisAny,
		"harness": fragment.AxisAny, "kind": fragment.KindRule,
	})

	if _, err := Apply(bs, partial, staticID); err == nil {
		t.Fatal("Apply succeeded with an unanswered batch — its lines would vanish silently")
	}
}

// A reviewer who cannot answer must be able to say so. Forcing a value would
// manufacture exactly the guess Confirm exists to prevent.
func TestApplySkipIsExplicitAndReported(t *testing.T) {
	bs := Batches(realProposals())
	answers := answerAll(bs, map[string]string{
		"host": fragment.AxisAny, "profile": fragment.AxisAny,
		"harness": fragment.AxisAny, "kind": fragment.KindRule,
	})
	answers[0] = Answer{Key: bs[0].Key(), Skip: true}

	res, err := Apply(bs, answers, staticID)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("got %d skipped batches, want 1 — a skip must be reported, not inferred from absence", len(res.Skipped))
	}
	for _, f := range res.Fragments {
		if strings.Contains(f.Source, bs[0].Path) && bs[0].Section != "" {
			for _, m := range bs[0].Members {
				if f.Source == m.Candidate.Origin() {
					t.Errorf("skipped batch still produced fragment %q", f.ID)
				}
			}
		}
	}
}

// An answer matching no batch means the corpus moved under a saved answer file.
// Applying the rest would tag lines from a decision made about different text.
func TestApplyRejectsStaleAnswers(t *testing.T) {
	bs := Batches(realProposals())
	answers := answerAll(bs, map[string]string{
		"host": fragment.AxisAny, "profile": fragment.AxisAny,
		"harness": fragment.AxisAny, "kind": fragment.KindRule,
	})
	answers = append(answers, Answer{Key: "/Users/c/.openclaw/workspace/GONE.md#Removed#profile", Values: map[string]string{"profile": "any"}})

	_, err := Apply(bs, answers, staticID)
	if err == nil {
		t.Fatal("Apply accepted an answer for a batch that does not exist — a stale decision would tag live lines")
	}
	if !strings.Contains(err.Error(), "GONE.md") {
		t.Errorf("error %q does not name the stale answer", err)
	}
}

// Apply must not launder a bad answer into a fragment: Confirm's validation is the
// last gate, and a batch answer is exactly where one wrong value reaches ten lines
// at once.
//
// The axis under test has to be one that is genuinely unresolved *and* drawn from a
// closed vocabulary, or the test proves nothing. An earlier version fed an invalid
// harness to batches whose harness was already resolved from the path — answerAll
// never carried the bad value, Apply never saw it, and the test failed for the right
// reason by luck. TOOLS.md leaves kind unresolved (it mixes rules and facts) and kind
// is a closed set, so this exercises the real gate.
func TestApplyPropagatesValidationFailure(t *testing.T) {
	ps := []Proposal{
		Propose(Candidate{
			Text: "Install policy: Homebrew first, UV for Python.",
			Path: "/Users/c/.openclaw/workspace/TOOLS.md", Line: 11, Section: "Conventions here",
		}, Options{Host: "m4-mini"}),
	}
	bs := Batches(ps)
	if len(bs) != 1 {
		t.Fatalf("got %d batches, want 1", len(bs))
	}
	if !contains(bs[0].Axes, "kind") {
		t.Fatalf("precondition: kind should be unresolved for a TOOLS.md line, axes = %v", bs[0].Axes)
	}

	values := map[string]string{}
	for _, axis := range bs[0].Axes {
		values[axis] = fragment.AxisAny
	}
	values["kind"] = "not-a-kind"

	if _, err := Apply(bs, []Answer{{Key: bs[0].Key(), Values: values}}, staticID); err == nil {
		t.Fatal("Apply accepted an invalid kind — a bad batch answer would reach every member unvalidated")
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestBatchReasonsSurviveDeduplication(t *testing.T) {
	ps := []Proposal{
		Propose(Candidate{Text: "Klaw orchestrates the fleet.", Path: "/Users/c/.openclaw/workspace/AGENTS.md", Line: 1, Section: "Role"},
			Options{Host: "m4-mini", Agents: []string{"klaw"}}),
		Propose(Candidate{Text: "Route work by capability need.", Path: "/Users/c/.openclaw/workspace/AGENTS.md", Line: 2, Section: "Role"},
			Options{Host: "m4-mini", Agents: []string{"klaw"}}),
	}
	bs := Batches(ps)
	if len(bs) != 1 {
		t.Fatalf("got %d batches, want 1", len(bs))
	}
	reasons := bs[0].Reasons()["profile"]
	if len(reasons) < 2 {
		t.Fatalf("profile reasons = %v; a line naming an agent and a line naming none are unresolved for different reasons, and the reviewer needs both", reasons)
	}
}

func staticID(p Proposal, b Batch, i int) string {
	return "f-" + itoa(len(p.Candidate.Text)) + "-" + itoa(b.Members[0].Candidate.Line) + "-" + itoa(i)
}
