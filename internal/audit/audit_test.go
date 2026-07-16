package audit

import (
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/compile"
	"github.com/cyperx84/soul-forge/internal/fragment"
)

func frag(id, text string) fragment.Fragment {
	return fragment.Fragment{
		ID: id, Text: text,
		Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
		Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
	}
}

func findByCheck(fs []Finding, check string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Check == check {
			out = append(out, f)
		}
	}
	return out
}

// Every check gets both halves asserted: it fires on the bad case AND stays silent
// on the clean one. A pass that flags nothing and a pass that flags everything both
// produce green ticks without the second half.

func TestDuplicateTextFiresAcrossEmphasisAndCase(t *testing.T) {
	corpus := []fragment.Fragment{
		frag("a", "**Install policy:** Homebrew first, UV for Python."),
		frag("b", "install policy: Homebrew first,   UV for Python."),
		frag("c", "A different rule entirely."),
	}
	got := findByCheck(Run(corpus, Options{}), "duplicate-fragment")
	if len(got) != 2 {
		t.Fatalf("want the pair flagged (2 findings), got %d: %v", len(got), got)
	}
	for _, f := range got {
		if f.FragmentID == "c" {
			t.Fatalf("clean fragment flagged as duplicate: %v", f)
		}
		if f.Severity != SeverityWarn {
			t.Fatalf("duplicate-fragment must be warn, got %s", f.Severity)
		}
	}
}

func TestDuplicateTextKeepsBacktickDistinction(t *testing.T) {
	// Code spans change meaning; `rm` the command and rm the word are not the same
	// sentence, and collapsing them would merge rules that differ where it counts.
	corpus := []fragment.Fragment{
		frag("a", "prefer `trash` over rm"),
		frag("b", "prefer trash over rm"),
	}
	if got := findByCheck(Run(corpus, Options{}), "duplicate-fragment"); len(got) != 0 {
		t.Fatalf("backtick-distinct texts must not merge, got %v", got)
	}
}

func TestProjectStateFiresOnDatesAndStatus(t *testing.T) {
	corpus := []fragment.Fragment{
		frag("dated", "Vault archived 2026-07-13; don't resurrect it."),
		frag("status", "M1 provisioning still pending."),
		frag("clean", "Secrets only in ~/.openclaw/secrets/."),
	}
	got := findByCheck(Run(corpus, Options{}), "project-state")
	if len(got) != 2 {
		t.Fatalf("want 2 project-state findings, got %d: %v", len(got), got)
	}
	for _, f := range got {
		if f.FragmentID == "clean" {
			t.Fatalf("dateless doctrine flagged as project state: %v", f)
		}
	}
}

func TestProjectStateIgnoresInstanceFragments(t *testing.T) {
	f := frag("mem", "Decided 2026-07-13: archived the vault.")
	f.Lifecycle = fragment.LifecycleInstance
	if got := findByCheck(Run([]fragment.Fragment{f}, Options{}), "project-state"); len(got) != 0 {
		t.Fatalf("instance fragments are runtime memory, dates there are fine: %v", got)
	}
}

func TestVagueLanguageRulesOnly(t *testing.T) {
	rule := frag("r", "Escalate as appropriate.")
	voice := frag("v", "Casual tone, swearing fine, etc.")
	voice.Kind = fragment.KindVoice
	got := findByCheck(Run([]fragment.Fragment{rule, voice}, Options{}), "vague-language")
	if len(got) != 1 || got[0].FragmentID != "r" {
		t.Fatalf("want only the rule flagged, got %v", got)
	}
}

func TestProvenanceFlagGated(t *testing.T) {
	corpus := []fragment.Fragment{
		frag("claim", "Sub-agent sessions only inject AGENTS.md and TOOLS.md."),
	}
	if got := findByCheck(Run(corpus, Options{}), "missing-provenance"); len(got) != 0 {
		t.Fatalf("provenance pass must be off by default, got %v", got)
	}
	got := findByCheck(Run(corpus, Options{Provenance: true}), "missing-provenance")
	if len(got) != 1 {
		t.Fatalf("want the uncited harness claim flagged, got %v", got)
	}
}

func TestProvenanceSatisfiedBySource(t *testing.T) {
	f := frag("cited", "Sub-agent sessions only inject AGENTS.md and TOOLS.md.")
	f.Source = "concepts/system-prompt.md:227"
	if got := findByCheck(Run([]fragment.Fragment{f}, Options{Provenance: true}), "missing-provenance"); len(got) != 0 {
		t.Fatalf("a cited claim must not be flagged, got %v", got)
	}
}

func targetsPair() []compile.Target {
	return []compile.Target{
		{Name: "hub", Selector: fragment.Selector{Host: "m4", Profile: "klaw", Harness: fragment.HarnessOpenClaw}},
		{Name: "cc", Selector: fragment.Selector{Host: "m4", Profile: "user", Harness: fragment.HarnessClaude}},
	}
}

func TestNarrowScopeCandidate(t *testing.T) {
	wide := frag("wide", "Reaches everything.") // any/any/any → both targets, not flagged
	narrow := frag("narrow", "Claims universality, reaches one place.")
	narrow.Harness = fragment.HarnessOpenClaw // host:any + profile:any but harness pins it to hub only
	got := findByCheck(Run([]fragment.Fragment{wide, narrow}, Options{Targets: targetsPair()}), "narrow-scope-candidate")
	if len(got) != 1 || got[0].FragmentID != "narrow" {
		t.Fatalf("want only the single-target any-fragment flagged, got %v", got)
	}
}

func TestNarrowScopeNeedsTwoTargets(t *testing.T) {
	f := frag("f", "Anything.")
	f.Harness = fragment.HarnessOpenClaw
	got := findByCheck(Run([]fragment.Fragment{f}, Options{Targets: targetsPair()[:1]}), "narrow-scope-candidate")
	if len(got) != 0 {
		t.Fatalf("one declared target makes the pass meaningless; it must stay silent, got %v", got)
	}
}

func TestBloatWarnsBeforeBudget(t *testing.T) {
	// 80% of a 1000-byte budget is 800; one fat fragment crosses it without
	// crossing the budget itself — the warning must land before the failure would.
	fat := frag("fat", strings.Repeat("x", 900))
	got := findByCheck(Run([]fragment.Fragment{fat}, Options{Targets: targetsPair()[:1], BudgetBytes: 1000}), "bloat")
	if len(got) != 1 {
		t.Fatalf("want a bloat warning at 80%% of budget, got %v", got)
	}
	slim := frag("slim", "short")
	if got := findByCheck(Run([]fragment.Fragment{slim}, Options{Targets: targetsPair()[:1], BudgetBytes: 1000}), "bloat"); len(got) != 0 {
		t.Fatalf("under-threshold file flagged, got %v", got)
	}
}

func TestCompileFailureIsAFindingNotACrash(t *testing.T) {
	bad := frag("secret", "key is sk-ant-abc123")
	got := findByCheck(Run([]fragment.Fragment{bad}, Options{Targets: targetsPair()[:1]}), "compile-failure")
	if len(got) != 1 {
		t.Fatalf("a corpus compile can't diagnose must surface as a finding, got %v", got)
	}
}

func TestOverridesReported(t *testing.T) {
	ov := fragment.Override{
		ID:           "disk",
		Parent:       frag("disk", "Disk fine."),
		Child:        frag("disk", "Disk chronically tight."),
		ChildProfile: "m4-mini",
	}
	got := findByCheck(Run(nil, Options{Overrides: []fragment.Override{ov}}), "override")
	if len(got) != 1 || got[0].Severity != SeverityInfo {
		t.Fatalf("override must be reported as info, got %v", got)
	}
}

func TestWarnSortsBeforeInfoAndHasWarnings(t *testing.T) {
	corpus := []fragment.Fragment{
		frag("vague", "Do it as appropriate."),             // info
		frag("dated", "Shipped 2026-07-13, still pending."), // warn
	}
	fs := Run(corpus, Options{})
	if len(fs) < 2 {
		t.Fatalf("expected both findings, got %v", fs)
	}
	if fs[0].Severity != SeverityWarn {
		t.Fatalf("warn must sort first, got %v", fs)
	}
	if !HasWarnings(fs) {
		t.Fatal("HasWarnings must be true when a warn exists")
	}
	if HasWarnings(findByCheck(fs, "vague-language")) {
		t.Fatal("info-only findings must not trip HasWarnings")
	}
}

func TestCleanCorpusIsClean(t *testing.T) {
	corpus := []fragment.Fragment{
		frag("a", "Secrets only in the secrets directory, never tracked files."),
		frag("b", "Prefer trash over rm; destructive ops must be restorable."),
	}
	if fs := Run(corpus, Options{Targets: targetsPair(), Provenance: true}); len(fs) != 0 {
		t.Fatalf("clean corpus produced findings: %v", fs)
	}
}
