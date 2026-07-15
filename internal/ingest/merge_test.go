package ingest

import (
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/compile"
	"github.com/cyperx84/soul-forge/internal/fragment"
)

// The corpus these tests run against is the real bug: "Install policy: Homebrew first"
// is byte-identical in the OpenClaw workspace's TOOLS.md and in ~/.claude/CLAUDE.md,
// which is the hand-sync CLAUDE.md's own header admits to in writing.

func toolsLine(text string, line int) Candidate {
	return Candidate{Text: text, Path: "/w/.openclaw/workspace/TOOLS.md", Line: line, Section: "Conventions here"}
}

func claudeLine(text string, line int) Candidate {
	return Candidate{Text: text, Path: "/w/.claude/CLAUDE.md", Line: line, Section: "Tooling"}
}

var fleet = []MergeTarget{
	{Name: "openclaw-hub", Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessOpenClaw}},
	{Name: "claude-global", Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessClaude}},
	{Name: "hermes", Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessHermes}},
	{Name: "codex", Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessCodex}},
}

// pair builds the Pair that Duplicates would produce for two candidates.
func pair(a, b Candidate, score float64) Pair {
	return Pair{A: a, B: b, Score: score, SameFile: a.Path == b.Path}
}

func opts() Options { return Options{Host: "m4-mini", Agents: []string{"klaw", "builder"}} }

// answerAll answers every non-refused question with merge=true, supplying nothing else.
func mergeAll(qs []MergeQuestion) []MergeAnswer {
	var out []MergeAnswer
	for _, q := range qs {
		if q.Refused != "" {
			continue
		}
		out = append(out, MergeAnswer{Key: q.Key, Merge: true})
	}
	return out
}

// TestMergeCollapsesTheHandSync is the payoff, asserted end to end: one authored line,
// two harnesses, one fragment. Before merge existed the corpus held this line twice and
// compile faithfully emitted the duplication.
func TestMergeCollapsesTheHandSync(t *testing.T) {
	const install = "Install policy: Homebrew first, UV for Python, else the project's recommended method."
	a := Propose(toolsLine(install, 12), opts())
	b := Propose(claudeLine(install, 88), opts())
	ps := []Proposal{a, b}

	qs := MergeQuestions([]Pair{pair(a.Candidate, b.Candidate, 1.0)}, ps, fleet)
	if len(qs) != 1 {
		t.Fatalf("got %d questions, want 1", len(qs))
	}
	if qs[0].NeedsText {
		t.Error("NeedsText on byte-identical lines — identical wording must merge without authoring")
	}
	if qs[0].Widens["harness"] == "" {
		t.Fatal("no harness widening reported: the merge's whole effect is unstated")
	}

	res, err := Merge(ps, qs, mergeAll(qs))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Proposals) != 1 {
		t.Fatalf("got %d proposals after merge, want 1 — the duplication survived", len(res.Proposals))
	}

	m := res.Proposals[0]
	if m.Harness.Value != fragment.AxisAny {
		t.Errorf("harness = %q, want %q — a merge that does not widen buys nothing", m.Harness.Value, fragment.AxisAny)
	}
	if !m.Harness.Certain {
		t.Error("merged harness is uncertain: the reviewer decided it, so it must not be asked again")
	}

	f, err := m.Confirm("install-policy", map[string]string{"host": fragment.AxisAny, "profile": fragment.AxisAny, "kind": fragment.KindRule})
	if err != nil {
		t.Fatal(err)
	}

	// The point of the exercise: one fragment, both outputs.
	for _, target := range []compile.Target{
		{Name: "openclaw-hub", Selector: fleet[0].Selector},
		{Name: "claude-global", Selector: fleet[1].Selector},
	} {
		out, err := compile.Compile([]fragment.Fragment{f}, target)
		if err != nil {
			t.Fatalf("%s: %v", target.Name, err)
		}
		var found bool
		for _, body := range out.Files {
			if strings.Contains(body, install) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: merged fragment did not render — the fragment authored once must reach both harnesses", target.Name)
		}
	}
}

// TestMergeReportsWhatTheWideningNewlyReaches pins the honest question. Two lines under
// openclaw and claude are evidence for those two harnesses; `any` is strictly more, and
// a reviewer who is not shown the difference is answering "are these the same?" while
// performing "should this reach Hermes?".
func TestMergeReportsWhatTheWideningNewlyReaches(t *testing.T) {
	const rule = "Never use exec/curl for provider messaging — OpenClaw routes internally."
	a := Propose(toolsLine(rule, 30), opts())
	b := Propose(claudeLine(rule, 30), opts())

	qs := MergeQuestions([]Pair{pair(a.Candidate, b.Candidate, 1.0)}, []Proposal{a, b}, fleet)
	blast := qs[0].Blast
	if len(blast) != 2 || !contains(blast, "hermes") || !contains(blast, "codex") {
		t.Fatalf("Blast = %v, want [codex hermes] — widening openclaw+claude to any reaches every harness, and this line is false for the ones it has never seen", blast)
	}
	for _, name := range []string{"openclaw-hub", "claude-global"} {
		if contains(blast, name) {
			t.Errorf("Blast names %s, which one of the lines already reaches — an alarm that fires on the status quo trains a reviewer to skim", name)
		}
	}
}

// TestNoWideningNoBlast: two lines already agreeing on every decided axis merge without
// claiming anything new, so there is nothing to warn about.
func TestNoWideningNoBlast(t *testing.T) {
	const rule = "trash > rm."
	a := Propose(toolsLine(rule, 5), opts())
	b := Propose(toolsLine(rule, 40), opts())

	qs := MergeQuestions([]Pair{pair(a.Candidate, b.Candidate, 1.0)}, []Proposal{a, b}, fleet)
	if len(qs[0].Widens) != 0 {
		t.Errorf("Widens = %v, want none: both lines are in one file, so no axis disagrees", qs[0].Widens)
	}
	if len(qs[0].Blast) != 0 {
		t.Errorf("Blast = %v, want none: nothing widened, so nothing is newly reached", qs[0].Blast)
	}
}

// TestUnansweredMergeDeclinesAndSaysSo. The default has to be safe, and it is: two
// fragments is today's state on disk. What is not safe is a step that silently does
// nothing and reports success — that is how "the dedup ran" gets believed.
func TestUnansweredMergeDeclinesAndSaysSo(t *testing.T) {
	const install = "Install policy: Homebrew first, UV for Python."
	a := Propose(toolsLine(install, 12), opts())
	b := Propose(claudeLine(install, 88), opts())
	ps := []Proposal{a, b}
	qs := MergeQuestions([]Pair{pair(a.Candidate, b.Candidate, 1.0)}, ps, fleet)

	res, err := Merge(ps, qs, nil)
	if err != nil {
		t.Fatal("unanswered merge questions must not be an error: forcing an answer to every ranked pair is how the migration gets abandoned; got " + err.Error())
	}
	if len(res.Proposals) != 2 {
		t.Fatalf("got %d proposals, want 2 — silence must never collapse a line nobody looked at", len(res.Proposals))
	}
	if len(res.Declined) != 1 {
		t.Fatalf("Declined = %d, want 1 — an unanswered question is a line left duplicated, and a merge step that reports nothing is indistinguishable from one that never ran", len(res.Declined))
	}
	if len(res.Merged) != 0 {
		t.Errorf("Merged = %v, want none", res.Merged)
	}
}

// TestExplicitDeclineIsNotAMerge: legitimate divergence is a real answer, not a
// failure. TOOLS.md's messaging rule and CLAUDE.md's are allowed to differ tomorrow.
func TestExplicitDeclineIsNotAMerge(t *testing.T) {
	a := Propose(toolsLine("Pin dependencies with an upper bound.", 9), opts())
	b := Propose(claudeLine("Pin dependencies with an upper bound.", 9), opts())
	ps := []Proposal{a, b}
	qs := MergeQuestions([]Pair{pair(a.Candidate, b.Candidate, 1.0)}, ps, fleet)

	res, err := Merge(ps, qs, []MergeAnswer{{Key: qs[0].Key, Merge: false}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Proposals) != 2 {
		t.Fatalf("got %d proposals, want 2 — a declined merge keeps both lines", len(res.Proposals))
	}
	if len(res.Declined) != 1 {
		t.Errorf("Declined = %d, want 1", len(res.Declined))
	}
}

// TestMergeRefusedAcrossLifecycle. Authored doctrine and runtime-written memory are not
// one fragment in any sense: instance fragments never compile, so merging either
// resurrects memory into the compiled corpus or drops an authored rule into a file
// compile never emits.
func TestMergeRefusedAcrossLifecycle(t *testing.T) {
	const text = "Fleet replication infra lives in fleet/."
	authored := Propose(toolsLine(text, 20), opts())
	memory := Propose(Candidate{Text: text, Path: "/w/.openclaw/workspace/MEMORY.md", Line: 8}, opts())
	if memory.Lifecycle.Value != fragment.LifecycleInstance {
		t.Fatal("precondition: MEMORY.md must propose lifecycle:instance")
	}
	ps := []Proposal{authored, memory}
	qs := MergeQuestions([]Pair{pair(authored.Candidate, memory.Candidate, 1.0)}, ps, fleet)

	if qs[0].Refused == "" {
		t.Fatal("an authored line and a line of runtime memory were offered as mergeable")
	}
	// The question is still emitted: a top-ranked pair vanishing with no explanation
	// reads as a ranking bug.
	if len(qs) != 1 {
		t.Fatalf("refused pair was dropped from the question list (%d questions)", len(qs))
	}

	if _, err := Merge(ps, qs, []MergeAnswer{{Key: qs[0].Key, Merge: true}}); err == nil {
		t.Fatal("Merge accepted a refused pair — a refusal a caller can override is a suggestion")
	}
}

// TestDivergentWordingMustBeAuthored. Two lines that mean the same thing in different
// words have no merged text until a human writes one; picking the longer, or A over B,
// is a machine authoring doctrine.
func TestDivergentWordingMustBeAuthored(t *testing.T) {
	a := Propose(toolsLine("Don't blur changed and verified — different states.", 3), opts())
	b := Propose(claudeLine("Don't claim done until verified from the user's perspective.", 3), opts())
	ps := []Proposal{a, b}
	qs := MergeQuestions([]Pair{pair(a.Candidate, b.Candidate, 0.14)}, ps, fleet)

	if !qs[0].NeedsText {
		t.Fatal("NeedsText false on differently-worded lines: the merged fragment would silently take one line's wording as doctrine")
	}
	if _, err := Merge(ps, qs, []MergeAnswer{{Key: qs[0].Key, Merge: true}}); err == nil {
		t.Fatal("Merge invented the text of a rule")
	}

	merged := "Don't blur changed, tested, verified, and delivered — and don't claim done until verified."
	res, err := Merge(ps, qs, []MergeAnswer{{Key: qs[0].Key, Merge: true, Text: merged}})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Proposals[0].Line(); got != merged {
		t.Errorf("Line() = %q, want the authored text", got)
	}
	// Provenance survives authoring: the primary candidate still reports what is
	// actually on that line of that file.
	if res.Proposals[0].Candidate.Text == merged {
		t.Error("authored text overwrote the candidate — Candidate is provenance and must keep reporting the real line")
	}
}

// TestMergedProvenanceNamesEveryOrigin. A merged fragment whose Source names one file
// hides that the rule was hand-synced across two — the exact thing being killed.
func TestMergedProvenanceNamesEveryOrigin(t *testing.T) {
	const install = "Install policy: Homebrew first."
	a := Propose(toolsLine(install, 12), opts())
	b := Propose(claudeLine(install, 88), opts())
	ps := []Proposal{a, b}
	qs := MergeQuestions([]Pair{pair(a.Candidate, b.Candidate, 1.0)}, ps, fleet)
	res, err := Merge(ps, qs, mergeAll(qs))
	if err != nil {
		t.Fatal(err)
	}

	f, err := res.Proposals[0].Confirm("install", map[string]string{"host": fragment.AxisAny, "profile": fragment.AxisAny, "kind": fragment.KindRule})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{a.Candidate.Origin(), b.Candidate.Origin()} {
		if !strings.Contains(f.Source, want) {
			t.Errorf("Source = %q, missing %s", f.Source, want)
		}
	}
	if len(res.Merged) != 1 || len(res.Merged[0].Origins) != 2 {
		t.Errorf("Merged = %+v, want one collapse tracing two origins", res.Merged)
	}
}

// TestTransitiveMergeIsOneGroup. Pairwise is how duplicates are found, not how they
// exist. Three copies surface as three pairs; applying them one at a time collapses the
// shared member twice and emits two fragments from three lines that are one.
func TestTransitiveMergeIsOneGroup(t *testing.T) {
	const rule = "Secrets never enter git or any tracked file."
	a := Propose(toolsLine(rule, 10), opts())
	b := Propose(claudeLine(rule, 20), opts())
	c := Propose(Candidate{Text: rule, Path: "/w/.hermes/SOUL.md", Line: 4}, opts())
	d := Propose(Candidate{Text: rule, Path: "/w/.codex/AGENTS.md", Line: 7}, opts())
	ps := []Proposal{a, b, c, d}

	// Order matters and this order is the point: two disjoint groups form first and a
	// later pair joins them. A merge that only walks a pair at a time, or a find that
	// stops one level short of the root, still passes a simple chain — it fails here,
	// emitting two fragments from four lines that are one rule.
	qs := MergeQuestions([]Pair{
		pair(a.Candidate, b.Candidate, 1.0),
		pair(c.Candidate, d.Candidate, 1.0),
		pair(b.Candidate, c.Candidate, 1.0),
	}, ps, fleet)

	res, err := Merge(ps, qs, mergeAll(qs))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Proposals) != 1 {
		t.Fatalf("got %d proposals, want 1 — three overlapping pairs are one group of four, not three merges", len(res.Proposals))
	}
	if got := len(res.Proposals[0].Origins()); got != 4 {
		t.Errorf("Origins() = %d, want 4 — every line the rule was copied to must be named", got)
	}
	if res.Proposals[0].Harness.Value != fragment.AxisAny {
		t.Errorf("harness = %q, want any", res.Proposals[0].Harness.Value)
	}
}

// TestCertaintyDoesNotSurviveAnUncertainMember. If one line's host was decided m4-mini
// and the other's was never decided, the merged host is unknown — not m4-mini. Guessing
// either way is role bleed one axis over: a machine fact pinned to the fleet, or a
// fleet rule that silently vanishes from every other box.
func TestCertaintyDoesNotSurviveAnUncertainMember(t *testing.T) {
	// AGENTS.md decides host:any (it is not the per-machine layer); CLAUDE.md cannot.
	decided := Propose(Candidate{Text: "Manual first; automate once understood.", Path: "/w/.openclaw/workspace/AGENTS.md", Line: 6}, opts())
	undecided := Propose(claudeLine("Manual first; automate once understood.", 60), opts())
	if !decided.Host.Certain || undecided.Host.Certain {
		t.Fatal("precondition: AGENTS.md decides host, CLAUDE.md does not")
	}
	ps := []Proposal{decided, undecided}
	qs := MergeQuestions([]Pair{pair(decided.Candidate, undecided.Candidate, 1.0)}, ps, fleet)

	if qs[0].Widens["host"] != "" {
		t.Error("host reported as widening: an undecided value is a placeholder, not a claim being given up")
	}

	res, err := Merge(ps, qs, mergeAll(qs))
	if err != nil {
		t.Fatal(err)
	}
	m := res.Proposals[0]
	if m.Host.Certain {
		t.Fatal("merged host came out certain — a merge with an undecided member cannot decide the axis")
	}
	if !contains(m.Unresolved(), "host") {
		t.Error("merged host is not unresolved: the question must survive into review, not be answered by the merge")
	}
	if _, err := m.Confirm("x", map[string]string{"profile": fragment.AxisAny}); err == nil {
		t.Fatal("Confirm built a fragment with an undecided host")
	}
}

// TestMergeAcrossKindsMustBeDecided. Kind has no `any`, so a disagreement cannot widen.
func TestMergeAcrossKindsMustBeDecided(t *testing.T) {
	voice := Propose(Candidate{Text: "Lead with the outcome.", Path: "/w/.openclaw/workspace/SOUL.md", Line: 4}, opts())
	rule := Propose(Candidate{Text: "Lead with the outcome.", Path: "/w/.openclaw/workspace/AGENTS.md", Line: 9}, opts())
	if voice.Kind.Value == rule.Kind.Value {
		t.Fatal("precondition: SOUL.md is voice, AGENTS.md is rule")
	}
	ps := []Proposal{voice, rule}
	qs := MergeQuestions([]Pair{pair(voice.Candidate, rule.Candidate, 1.0)}, ps, fleet)

	if !qs[0].NeedsKind {
		t.Fatal("NeedsKind false across voice and rule — the merge would silently keep whichever came first")
	}
	if _, err := Merge(ps, qs, []MergeAnswer{{Key: qs[0].Key, Merge: true}}); err == nil {
		t.Fatal("Merge picked a kind for the reviewer")
	}

	res, err := Merge(ps, qs, []MergeAnswer{{Key: qs[0].Key, Merge: true, Kind: fragment.KindRule}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Proposals[0].Kind.Value != fragment.KindRule {
		t.Errorf("kind = %q, want the reviewer's answer", res.Proposals[0].Kind.Value)
	}
}

// TestConflictingAnswersInOneGroup. A group is one fragment and one fragment has one
// line; two humans authoring different text for the same group has no defensible
// tiebreak, and picking either discards a decision someone made.
func TestConflictingAnswersInOneGroup(t *testing.T) {
	a := Propose(toolsLine("Prefer jq over python -c.", 1), opts())
	b := Propose(claudeLine("Prefer shell pipelines over perl -e.", 2), opts())
	c := Propose(Candidate{Text: "Use awk, not python one-liners.", Path: "/w/.hermes/SOUL.md", Line: 3}, opts())
	ps := []Proposal{a, b, c}
	qs := MergeQuestions([]Pair{
		pair(a.Candidate, b.Candidate, 0.6),
		pair(b.Candidate, c.Candidate, 0.6),
	}, ps, fleet)

	_, err := Merge(ps, qs, []MergeAnswer{
		{Key: qs[0].Key, Merge: true, Text: "Prefer jq/sed/awk over python -c and perl -e.", Kind: fragment.KindRule},
		{Key: qs[1].Key, Merge: true, Text: "Use awk.", Kind: fragment.KindRule},
	})
	if err == nil {
		t.Fatal("Merge silently picked one of two conflicting authored lines")
	}
	if !strings.Contains(err.Error(), "conflicting") {
		t.Errorf("error = %q, want it to name the conflict", err)
	}
}

// TestCollapseBackstopsAMissingDecision drives collapse directly.
//
// Merge checks NeedsText and NeedsKind before it ever gets here, so these two branches
// are unreachable through the public path — and a mutation that deletes them survives
// the whole suite, which is how they earned a direct test rather than a shrug. They are
// the same class as collapse's lifecycle re-check: an invariant holding by construction
// today is the one a later refactor breaks silently, and the thing being defended is a
// machine authoring a rule nobody wrote.
func TestCollapseBackstopsAMissingDecision(t *testing.T) {
	a := Propose(toolsLine("Prefer jq over python -c.", 1), opts())
	b := Propose(claudeLine("Use shell pipelines, not perl -e.", 2), opts())
	members := []Proposal{a, b}

	p := pair(a.Candidate, b.Candidate, 0.5)
	q := MergeQuestion{Key: mergeKey(p), Pair: p, NeedsText: true}
	byKey := map[string]MergeQuestion{q.Key: q}

	// An answer that passed the upfront check but carries no text: what a refactor
	// dropping that check would produce.
	accepted := map[string]MergeAnswer{q.Key: {Key: q.Key, Merge: true}}

	if _, err := collapse(members, byKey, accepted); err == nil {
		t.Fatal("collapse invented the text of a rule from two differently-worded lines")
	}

	// Same shape for kind, which has no wildcard to fall back on.
	voice := Propose(Candidate{Text: "Lead with the outcome.", Path: "/w/.openclaw/workspace/SOUL.md", Line: 4}, opts())
	rule := Propose(Candidate{Text: "Lead with the outcome.", Path: "/w/.openclaw/workspace/AGENTS.md", Line: 9}, opts())
	kp := pair(voice.Candidate, rule.Candidate, 1.0)
	kq := MergeQuestion{Key: mergeKey(kp), Pair: kp, NeedsKind: true}
	if _, err := collapse([]Proposal{voice, rule}, map[string]MergeQuestion{kq.Key: kq},
		map[string]MergeAnswer{kq.Key: {Key: kq.Key, Merge: true}}); err == nil {
		t.Fatal("collapse picked a kind across voice and rule with no answer stating one")
	}
}

// TestUnknownAnswerIsAnError. An answer matching no question means the corpus moved
// under a saved answer file — the same failure Apply refuses, for the same reason: a
// decision made about different text would collapse lines nobody looked at.
func TestUnknownAnswerIsAnError(t *testing.T) {
	a := Propose(toolsLine("x y z.", 1), opts())
	b := Propose(claudeLine("x y z.", 2), opts())
	ps := []Proposal{a, b}
	qs := MergeQuestions([]Pair{pair(a.Candidate, b.Candidate, 1.0)}, ps, fleet)

	if _, err := Merge(ps, qs, []MergeAnswer{{Key: "/w/gone.md:1~/w/also-gone.md:2", Merge: true}}); err == nil {
		t.Fatal("Merge accepted an answer for a question that does not exist")
	}
}

// TestMergeIsIdentityWithoutAnswers is the inverse assertion. Every test above would
// also pass against a Merge that collapsed the whole corpus into one fragment, and
// that merger is worthless.
func TestMergeIsIdentityWithoutAnswers(t *testing.T) {
	ps := []Proposal{
		Propose(toolsLine("Homebrew first.", 1), opts()),
		Propose(claudeLine("Homebrew first.", 2), opts()),
		Propose(toolsLine("Never tmux kill-server.", 3), opts()),
	}
	res, err := Merge(ps, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Proposals) != len(ps) {
		t.Fatalf("got %d proposals, want %d — no questions and no answers must change nothing", len(res.Proposals), len(ps))
	}
	for i := range ps {
		if res.Proposals[i].Candidate.Origin() != ps[i].Candidate.Origin() {
			t.Errorf("proposal %d reordered: %s", i, res.Proposals[i].Candidate.Origin())
		}
		if len(res.Proposals[i].Absorbed) != 0 {
			t.Errorf("proposal %d absorbed something with no answer", i)
		}
	}
}

// TestUnrelatedLinesNeverMerge. Answering the questions asked must not collapse lines
// no question was asked about.
func TestUnrelatedLinesNeverMerge(t *testing.T) {
	dupA := Propose(toolsLine("Homebrew first.", 1), opts())
	dupB := Propose(claudeLine("Homebrew first.", 2), opts())
	other := Propose(toolsLine("Never tmux kill-server on the default socket.", 3), opts())
	ps := []Proposal{dupA, dupB, other}

	qs := MergeQuestions([]Pair{pair(dupA.Candidate, dupB.Candidate, 1.0)}, ps, fleet)
	res, err := Merge(ps, qs, mergeAll(qs))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Proposals) != 2 {
		t.Fatalf("got %d proposals, want 2 (one merged, one untouched)", len(res.Proposals))
	}
	var found bool
	for _, p := range res.Proposals {
		if p.Candidate.Origin() == other.Candidate.Origin() {
			found = true
			if len(p.Absorbed) != 0 {
				t.Error("an unrelated line was absorbed into a merge")
			}
		}
	}
	if !found {
		t.Error("the unrelated line vanished")
	}
}

// TestMergedFlagsSurvive. A possible secret does not stop being one because the line it
// matched turned out to live in two files.
func TestMergedFlagsSurvive(t *testing.T) {
	const key = "Never commit sk-abcdefghij0123456789 to the repo."
	a := Propose(toolsLine(key, 1), opts())
	b := Propose(claudeLine(key, 2), opts())
	if len(a.Flags) == 0 {
		t.Fatal("precondition: the secret-shaped line must raise a flag")
	}
	ps := []Proposal{a, b}
	qs := MergeQuestions([]Pair{pair(a.Candidate, b.Candidate, 1.0)}, ps, fleet)
	res, err := Merge(ps, qs, mergeAll(qs))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Proposals[0].Flags) == 0 {
		t.Error("merging dropped the flags — a merged line is more visible, not less")
	}
}

// TestMergeQuestionKeysAreStable. Answers are written to a file and replayed; a key
// that moves between runs turns every saved answer into the "corpus changed" error.
func TestMergeQuestionKeysAreStable(t *testing.T) {
	a := Propose(toolsLine("Homebrew first.", 1), opts())
	b := Propose(claudeLine("Homebrew first.", 2), opts())
	ps := []Proposal{a, b}
	p := pair(a.Candidate, b.Candidate, 1.0)

	first := MergeQuestions([]Pair{p}, ps, fleet)
	second := MergeQuestions([]Pair{p}, ps, fleet)
	if first[0].Key != second[0].Key {
		t.Fatalf("key changed across runs: %q vs %q", first[0].Key, second[0].Key)
	}
	if !strings.Contains(first[0].Key, "TOOLS.md:1") || !strings.Contains(first[0].Key, "CLAUDE.md:2") {
		t.Errorf("key = %q, want it to name both origins so a saved answer is readable", first[0].Key)
	}
}
