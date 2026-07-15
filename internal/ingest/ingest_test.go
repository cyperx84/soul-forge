package ingest

import (
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

// The corpus throughout these tests is the real thing: lines lifted from the Klaw
// workspace rewrite, including the three duplicates two LLM cross-review rounds found
// by hand. Invented fixtures would only prove the code agrees with itself.

func TestExtractSkipsStructureAndFences(t *testing.T) {
	src := "# AGENTS.md\n\n" +
		"## Red lines\n\n" +
		"- `trash` > `rm`. Inspect-and-preserve before changing config.\n" +
		"\n" +
		"```sh\n" +
		"rm -rf /\n" +
		"```\n" +
		"| axis | values |\n" +
		"|---|---|\n" +
		"> quoted aside\n" +
		"<!-- a comment -->\n" +
		"1. Ordered item text.\n"

	got, err := Extract("AGENTS.md", strings.NewReader(src))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(got) != 2 {
		for _, c := range got {
			t.Logf("candidate %s: %q", c.Origin(), c.Text)
		}
		t.Fatalf("got %d candidates, want 2 (the bullet and the ordered item)", len(got))
	}

	// A fenced command must never become a rule. Ingesting `rm -rf /` as doctrine
	// would invert a red line — the worst thing this package could do.
	for _, c := range got {
		if strings.Contains(c.Text, "rm -rf") {
			t.Fatalf("fenced code became a candidate: %q", c.Text)
		}
	}

	if got[0].Section != "Red lines" {
		t.Errorf("section = %q, want %q", got[0].Section, "Red lines")
	}
	if got[0].Line != 5 {
		t.Errorf("line = %d, want 5", got[0].Line)
	}
	if !strings.HasPrefix(got[0].Text, "`trash` > `rm`") {
		t.Errorf("list marker not stripped: %q", got[0].Text)
	}
	if got[1].Text != "Ordered item text." {
		t.Errorf("ordered marker not stripped: %q", got[1].Text)
	}
	if got[0].Origin() != "AGENTS.md:5" {
		t.Errorf("Origin = %q, want AGENTS.md:5", got[0].Origin())
	}
}

func TestExtractIgnoresEmptyInput(t *testing.T) {
	got, err := Extract("SOUL.md", strings.NewReader("# SOUL.md\n\n"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d candidates from a headings-only file, want 0", len(got))
	}
}

func TestProposeFilenameDecidesKind(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/w/SOUL.md", fragment.KindVoice},
		{"/w/AGENTS.md", fragment.KindRule},
		{"/w/IDENTITY.md", fragment.KindIdentity},
		{"/w/USER.md", fragment.KindFact},
	}
	for _, tc := range cases {
		p := Propose(Candidate{Text: "Lead with the outcome.", Path: tc.path, Line: 1}, Options{Host: "m4-mini"})
		if !p.Kind.Certain {
			t.Errorf("%s: kind unresolved, want certain (the harness docs assign this file a role)", tc.path)
		}
		if p.Kind.Value != tc.want {
			t.Errorf("%s: kind = %q, want %q", tc.path, p.Kind.Value, tc.want)
		}
	}
}

// TOOLS.md and CLAUDE.md genuinely mix kinds, and the honest output says so rather
// than defaulting. If this test ever "passes" by proposing a certain kind, ingest has
// started guessing.
func TestProposeRefusesToDecideMixedFiles(t *testing.T) {
	for _, path := range []string{"/w/TOOLS.md", "/home/.claude/CLAUDE.md"} {
		p := Propose(Candidate{Text: "Install policy: Homebrew first, UV for Python.", Path: path}, Options{Host: "m4-mini"})
		if p.Kind.Certain {
			t.Errorf("%s: kind reported certain — this file mixes rules and facts; deciding it is a guess", path)
		}
	}
}

// The per-machine layer tempts a host:<this box> default. That default is wrong for
// the harness rules living in TOOLS.md, and pinning a fleet-wide rule to one machine
// deletes it from every other box — role bleed, one axis over.
func TestProposeDecidesHostForNonMachineLayerFiles(t *testing.T) {
	// AGENTS.md is documented as not the per-machine layer, so host:any is decided
	// by the file's role — the same class of evidence that makes SOUL.md voice.
	// Refusing here would hand a reviewer every line in the corpus with one answer.
	p := Propose(Candidate{Text: "Act first, explain after.", Path: "/w/AGENTS.md"}, Options{Host: "m4-mini"})
	if !p.Host.Certain {
		t.Errorf("host unresolved for an AGENTS.md line with no machine in it: %s", p.Host.Reason)
	}
	if p.Host.Value != fragment.AxisAny {
		t.Errorf("host = %q, want %q", p.Host.Value, fragment.AxisAny)
	}
}

// A line naming hardware needs a human whatever file holds it: the machine id is the
// caller's to state, and host:any on a one-box fact would pin it to the fleet.
func TestProposeRefusesHostForHardwareLines(t *testing.T) {
	p := Propose(Candidate{
		Text: "Apple Silicon M4 Mac Mini, 32GB RAM, 228GB SSD — disk chronically tight.",
		Path: "/w/AGENTS.md",
	}, Options{Host: "m4-mini"})
	if p.Host.Certain {
		t.Fatal("host reported certain for a line naming hardware — one box's fact would compile to every machine")
	}
	if p.Host.Value != "m4-mini" {
		t.Errorf("host proposal = %q, want the caller-supplied machine id as the suggestion", p.Host.Value)
	}
}

func TestProposeNeverDefaultsHostForToolsFile(t *testing.T) {
	p := Propose(Candidate{
		Text: "Never use exec/curl for provider messaging — OpenClaw routes internally.",
		Path: "/w/TOOLS.md",
	}, Options{Host: "m4-mini"})

	if p.Host.Certain {
		t.Fatal("host reported certain for a TOOLS.md line: a harness rule filed under a machine would be pinned to one box")
	}
	if p.Host.Value != fragment.AxisAny {
		t.Errorf("host = %q, want %q as the unresolved placeholder", p.Host.Value, fragment.AxisAny)
	}
}

func TestProposeMemoryIsInstanceLifecycle(t *testing.T) {
	p := Propose(Candidate{Text: "Fleet vault archived.", Path: "/w/MEMORY.md"}, Options{})
	if p.Lifecycle.Value != fragment.LifecycleInstance || !p.Lifecycle.Certain {
		t.Fatalf("MEMORY.md lifecycle = %q (certain=%v), want %q certain",
			p.Lifecycle.Value, p.Lifecycle.Certain, fragment.LifecycleInstance)
	}
}

func TestProposeHarnessFromPath(t *testing.T) {
	cases := map[string]string{
		"/Users/c/.openclaw/workspace/AGENTS.md": fragment.HarnessOpenClaw,
		"/Users/c/.claude/CLAUDE.md":             fragment.HarnessClaude,
		"/Users/c/.hermes/SOUL.md":               fragment.HarnessHermes,
		"/Users/c/.codex/AGENTS.md":              fragment.HarnessCodex,
	}
	for path, want := range cases {
		p := Propose(Candidate{Text: "some rule", Path: path}, Options{})
		if p.Harness.Value != want || !p.Harness.Certain {
			t.Errorf("%s: harness = %q (certain=%v), want %q certain", path, p.Harness.Value, p.Harness.Certain, want)
		}
	}
}

// A named agent is ambiguous by construction: "Klaw orchestrates the fleet" is about
// Klaw, "Klaw: act first" is addressed to Klaw, and the two want different profile
// tags. This is the line that broke matrix v2 — deciding it from wording is what
// ingest must not do.
func TestProposeFlagsNamedAgentWithoutDeciding(t *testing.T) {
	p := Propose(Candidate{
		Text: "Klaw orchestrates the fleet from this machine.",
		Path: "/w/AGENTS.md",
	}, Options{Host: "m4-mini", Agents: []string{"klaw", "builder"}})

	if p.Profile.Certain {
		t.Fatal("profile reported certain for a line naming an agent — this is the matrix-v2 break; it needs a human")
	}
	if !strings.Contains(p.Profile.Reason, "klaw") {
		t.Errorf("reason %q does not name the agent it spotted", p.Profile.Reason)
	}
}

// The structural guarantee: an unreviewed proposal cannot become a fragment. There is
// no other path from Proposal to Fragment, so "compiled a guess" is impossible rather
// than discouraged.
func TestConfirmRefusesUnresolvedAxis(t *testing.T) {
	p := Propose(Candidate{Text: "Install policy: Homebrew first.", Path: "/w/TOOLS.md"}, Options{Host: "m4-mini"})
	if len(p.Unresolved()) == 0 {
		t.Fatal("precondition failed: TOOLS.md line should have unresolved axes")
	}

	if _, err := p.Confirm("install-policy", nil); err == nil {
		t.Fatal("Confirm succeeded with unresolved axes — an unreviewed guess reached the corpus")
	}

	// Supplying every unresolved axis is the review step; then it builds.
	overrides := map[string]string{}
	for _, axis := range p.Unresolved() {
		switch axis {
		case "host":
			overrides["host"] = fragment.AxisAny
		case "profile":
			overrides["profile"] = fragment.AxisAny
		case "harness":
			overrides["harness"] = fragment.AxisAny
		case "lifecycle":
			overrides["lifecycle"] = fragment.LifecycleAuthored
		case "kind":
			overrides["kind"] = fragment.KindRule
		}
	}
	f, err := p.Confirm("install-policy", overrides)
	if err != nil {
		t.Fatalf("Confirm with every axis supplied: %v", err)
	}
	if f.Source != "/w/TOOLS.md:0" {
		t.Errorf("Source = %q, want the origin line — provenance is the point", f.Source)
	}
	if f.Text != "Install policy: Homebrew first." {
		t.Errorf("Text = %q, mangled", f.Text)
	}
}

// A signal can be wrong. A reviewer overruling a certain axis is the system working,
// not a violation — the failure mode is the machine deciding unasked, not the human
// deciding differently.
func TestConfirmAllowsOverridingACertainAxis(t *testing.T) {
	p := Propose(Candidate{Text: "Lead with the outcome.", Path: "/w/SOUL.md"}, Options{Host: "m4-mini"})
	f, err := p.Confirm("lead-outcome", map[string]string{
		"kind": fragment.KindRule, // overruling the SOUL.md → voice signal
		"host": fragment.AxisAny, "profile": fragment.AxisAny, "harness": fragment.AxisAny,
	})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if f.Kind != fragment.KindRule {
		t.Errorf("kind = %q, want the override %q to win over the filename signal", f.Kind, fragment.KindRule)
	}
}

func TestFlagsCatchWhatAlreadyShippedOnce(t *testing.T) {
	cases := []struct {
		name, text, want string
	}{
		{"model name", "Route Claude handoffs to claude-opus-4-8.", "names a model"},
		{"secret", "export KEY=sk-abcdefghijklmnopqrstuvwx", "possible secret"},
		{"runtime contract", "If no response is needed, reply NO_REPLY.", "names runtime-injected contract"},
	}
	for _, tc := range cases {
		p := Propose(Candidate{Text: tc.text, Path: "/w/AGENTS.md"}, Options{Host: "m4-mini"})
		if !hasFlag(p.Flags, tc.want) {
			t.Errorf("%s: flags = %v, want one containing %q", tc.name, p.Flags, tc.want)
		}
	}
}

func hasFlag(flags []string, want string) bool {
	for _, f := range flags {
		if strings.Contains(f, want) {
			return true
		}
	}
	return false
}

// realDuplicates is the exam. These are the three duplicate pairs that two rounds of
// LLM cross-review found by hand in the Klaw files — the spec's acceptance test says
// ingest must surface them independently, or the model is wrong again.
func realDuplicates() []Candidate {
	return []Candidate{
		// Pair 1: the diagram preference, stated in both USER.md and SOUL.md.
		{Text: "Prefers diagrams for complex or architectural explanations.", Path: "USER.md", Line: 20},
		{Text: "Lead with a diagram when the thing is architectural — Chris reads those faster than prose.", Path: "SOUL.md", Line: 9},

		// Pair 2: done-vs-attempted, stated in both SOUL.md and AGENTS.md.
		{Text: "Don't fake certainty. Own misses plainly; never blur done with attempted.", Path: "SOUL.md", Line: 8},
		{Text: "Don't blur changed / tested / verified / delivered — different states. Don't claim done until verified.", Path: "AGENTS.md", Line: 22},

		// Pair 3: ask-before-destructive, stated twice inside AGENTS.md itself.
		{Text: "Confirm only for: destructive actions, anything leaving the machine, or infra-critical changes.", Path: "AGENTS.md", Line: 21},
		{Text: "Ask before destructive or outward-facing actions.", Path: "AGENTS.md", Line: 48},

		// Noise: unrelated rules that must not outrank the real pairs.
		{Text: "Secrets only in ~/.openclaw/secrets — never in git, config, or any tracked file.", Path: "AGENTS.md", Line: 45},
		{Text: "Install policy: Homebrew first, UV for Python, else the project's recommended method.", Path: "TOOLS.md", Line: 12},
		{Text: "Fleet connects over Tailscale.", Path: "TOOLS.md", Line: 7},
		{Text: "Each session starts fresh; these files are continuity.", Path: "AGENTS.md", Line: 15},
	}
}

// The exam, asserted the only way the score allows: by rank.
//
// The three duplicates two LLM cross-review rounds found by hand must be the three
// top-ranked pairs — not "above some number", because the number moves with corpus
// size. An earlier version of this test asserted score >= 0.15 and failed while the
// ranking was already perfect, which is what exposed the threshold as the bug.
func TestDuplicatesReproducesTheHandFoundMap(t *testing.T) {
	pairs := Duplicates(realDuplicates(), FloorDefault)

	handFound := []struct{ a, b string }{
		{"USER.md:20", "SOUL.md:9"},      // diagrams: USER states it, SOUL restates it
		{"SOUL.md:8", "AGENTS.md:22"},    // done vs attempted
		{"AGENTS.md:21", "AGENTS.md:48"}, // ask before destructive, twice inside AGENTS
	}
	if len(pairs) < len(handFound) {
		t.Fatalf("got %d pairs, want at least the %d the hand review found", len(pairs), len(handFound))
	}

	top := pairs[:len(handFound)]
	for _, w := range handFound {
		if !foundPair(top, w.a, w.b) {
			t.Errorf("hand-found duplicate not in the top %d: %s <-> %s", len(handFound), w.a, w.b)
		}
	}
	if t.Failed() {
		for i, p := range pairs {
			t.Logf("#%d %.4f %s <-> %s shared=%v", i+1, p.Score, p.A.Origin(), p.B.Origin(), p.Shared)
		}
	}
}

// The separation, not just the order. If the real duplicates only barely outrank the
// noise, the ranking is luck and the next corpus reshuffles it. A reviewer reading
// top-down needs a visible place to stop.
func TestDuplicatesSeparateRealFromNoise(t *testing.T) {
	pairs := Duplicates(realDuplicates(), FloorDefault)
	if len(pairs) < 4 {
		t.Fatalf("got %d pairs, need at least 4 to compare real against noise", len(pairs))
	}
	lastReal, firstNoise := pairs[2].Score, pairs[3].Score
	if lastReal < firstNoise*1.5 {
		t.Errorf("weakest real duplicate scored %.4f, strongest noise %.4f — no clear place to stop reading",
			lastReal, firstNoise)
	}
}

// Both duplication shapes matter and they are different bugs: cross-file is hand-sync
// drift between two owners, same-file is a rule restated inside its own owner. The
// hand review found one of each, so reporting only one shape would fail the exam
// while looking like it passed.
func TestDuplicatesReportsBothShapes(t *testing.T) {
	pairs := Duplicates(realDuplicates(), FloorDefault)
	var cross, same bool
	for _, p := range pairs {
		if p.SameFile {
			same = true
		} else {
			cross = true
		}
	}
	if !cross {
		t.Error("no cross-file pair reported: hand-sync drift between two files would go unseen")
	}
	if !same {
		t.Error("no same-file pair reported: a rule restated inside its own owner would go unseen")
	}
}

// The method's whole claim is that shared *rare* wording beats shared *common*
// wording. Both pairs below share exactly two terms out of four, so plain overlap —
// Jaccard, shared-word count, any unweighted measure — scores them identically and
// cannot tell them apart. Only IDF can: "diagram" appearing twice in a corpus is
// remarkable, "check" appearing in most of it is not.
//
// If this test ever passes without IDF, IDF is dead weight. If it fails, real
// duplicates drown in every pair that happens to share "check … before".
func TestRareTermsOutrankCommonTerms(t *testing.T) {
	cands := []Candidate{
		// Rare pair: shares diagram + architectural, found nowhere else.
		{Text: "Prefers diagrams for architectural explanations.", Path: "USER.md", Line: 1},
		{Text: "Lead with a diagram when architectural.", Path: "SOUL.md", Line: 2},
		// Common pair: shares check + before, which saturate the corpus below.
		{Text: "Check permissions before deleting.", Path: "AGENTS.md", Line: 3},
		{Text: "Check quotas before uploading.", Path: "AGENTS.md", Line: 4},
		// Filler driving check/before to a high document frequency, each otherwise
		// sharing no vocabulary with anything.
		{Text: "Check batteries before travelling.", Path: "AGENTS.md", Line: 5},
		{Text: "Check receipts before invoicing.", Path: "AGENTS.md", Line: 6},
		{Text: "Check tyres before driving.", Path: "AGENTS.md", Line: 7},
		{Text: "Check passports before flying.", Path: "AGENTS.md", Line: 8},
	}
	pairs := Duplicates(cands, 0.0)
	if len(pairs) == 0 {
		t.Fatal("no pairs at floor 0 — scoring is dead")
	}

	rare := findPair(pairs, "USER.md:1", "SOUL.md:2")
	common := findPair(pairs, "AGENTS.md:3", "AGENTS.md:4")
	if rare == nil {
		t.Fatal("rare-term pair not scored at all")
	}
	if common == nil {
		t.Fatal("common-term pair not scored at all — precondition for the comparison")
	}
	if rare.Score <= common.Score {
		t.Fatalf("rare-term pair scored %.4f, common-term pair %.4f — both share 2 of 4 terms, so an unweighted measure ties them; IDF is not discriminating",
			rare.Score, common.Score)
	}
	if pairs[0].A.Origin() != "USER.md:1" || pairs[0].B.Origin() != "SOUL.md:2" {
		t.Errorf("top pair is %s <-> %s; want the rare-term pair ranked first",
			pairs[0].A.Origin(), pairs[0].B.Origin())
	}
}

// A threshold that lets everything through is not a detector, and one that lets
// nothing through passes every "found no duplicates" assertion while being useless.
// Pin both directions, the same way the role-bleed test pins over-filtering.
func TestFloorBoundsTheListWithoutJudging(t *testing.T) {
	cands := realDuplicates()
	if got := Duplicates(cands, 1.01); len(got) != 0 {
		t.Errorf("floor above 1 returned %d pairs, want 0", len(got))
	}
	loose := Duplicates(cands, 0.0)
	tight := Duplicates(cands, 0.5)
	if len(loose) <= len(tight) {
		t.Errorf("floor 0 returned %d pairs, floor 0.5 returned %d — floor is not bounding the list", len(loose), len(tight))
	}
}

// FloorDefault must never cut a real duplicate. It bounds list length; the moment it
// starts making similarity judgments it is a threshold again, and the threshold is
// what cut the done-vs-attempted pair the first time.
func TestFloorDefaultKeepsEveryHandFoundDuplicate(t *testing.T) {
	pairs := Duplicates(realDuplicates(), FloorDefault)
	for _, w := range []struct{ a, b string }{
		{"USER.md:20", "SOUL.md:9"},
		{"SOUL.md:8", "AGENTS.md:22"},
		{"AGENTS.md:21", "AGENTS.md:48"},
	} {
		if !foundPair(pairs, w.a, w.b) {
			t.Errorf("FloorDefault cut a real duplicate: %s <-> %s", w.a, w.b)
		}
	}
}

func TestDuplicatesIsDeterministic(t *testing.T) {
	a := Duplicates(realDuplicates(), 0.15)
	b := Duplicates(realDuplicates(), 0.15)
	if len(a) != len(b) {
		t.Fatalf("run 1 gave %d pairs, run 2 gave %d", len(a), len(b))
	}
	for i := range a {
		if a[i].A.Origin() != b[i].A.Origin() || a[i].B.Origin() != b[i].B.Origin() {
			t.Fatalf("order differs at %d: %s/%s vs %s/%s — output must be diffable across runs",
				i, a[i].A.Origin(), a[i].B.Origin(), b[i].A.Origin(), b[i].B.Origin())
		}
	}
}

func findPair(pairs []Pair, a, b string) *Pair {
	for i := range pairs {
		x, y := pairs[i].A.Origin(), pairs[i].B.Origin()
		if (x == a && y == b) || (x == b && y == a) {
			return &pairs[i]
		}
	}
	return nil
}

func foundPair(pairs []Pair, a, b string) bool {
	for _, p := range pairs {
		x, y := p.A.Origin(), p.B.Origin()
		if (x == a && y == b) || (x == b && y == a) {
			return true
		}
	}
	return false
}
