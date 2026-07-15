// The role-bleed regression test.
//
// V2-SPEC.md pinned it as build step 1 and it shipped red, before the compiler
// existed: it is the regression test for the break that killed the hand-built
// ownership matrix, and it is the reason the data model changed. Writing it first
// meant the compiler was judged against it rather than negotiated with it. Step 4
// made it green.
//
// It stays green only while the compiler refuses to leak scope, and never by
// weakening what it asserts.

package compile_test

import (
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/compile"
	"github.com/cyperx84/soul-forge/internal/fragment"
)

// corpus mirrors the real lines that broke the hand-built matrix. Each one is a
// documented failure from fleet/agent-files-rewrite.md, not an invented fixture.
func corpus() []fragment.Fragment {
	return []fragment.Fragment{
		{
			// The line that broke matrix v2: it sat in a file declared portable, and
			// survived a clone to another machine but not to another role.
			ID: "klaw-orchestrates", Text: "Klaw orchestrates the fleet.",
			Host: fragment.AxisAny, Profile: "klaw", Harness: fragment.HarnessOpenClaw,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindIdentity,
		},
		{
			// Machine-local fact. Renders to TOOLS.md on OpenClaw and to a CLAUDE.md
			// section on Claude Code — one fragment, two outputs, no hand-sync.
			ID: "m4-disk-tight", Text: "Disk chronically tight (228GB, often 90%+).",
			Host: "m4-mini", Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact,
		},
		{
			// Universal doctrine: every output, every machine, one source.
			ID: "trash-not-rm", Text: "trash > rm.",
			Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
		},
		{
			// OpenClaw-only mechanics. Must not reach a Claude target.
			ID: "sessions-yield", Text: "Wait with sessions_yield — never busy-poll.",
			Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.HarnessOpenClaw,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
		},
		{
			ID: "builder-builds", Text: "Builder implements; it does not route work.",
			Host: fragment.AxisAny, Profile: "builder", Harness: fragment.HarnessOpenClaw,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindIdentity,
		},
		{
			// Runtime memory. Compile must never emit it, whatever else it matches.
			ID: "wiki-archived", Text: "The fleet wiki went stale-dump and got archived.",
			Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleInstance, Kind: fragment.KindFact,
		},
	}
}

func builderOnM1() compile.Target {
	return compile.Target{
		Name: "openclaw-worker",
		Selector: fragment.Selector{
			Host: "m1", Profile: "builder", Harness: fragment.HarnessOpenClaw,
		},
	}
}

// TestRoleBleed is the invariant: compiling one agent on one machine must not leak
// another agent's identity or another machine's facts. This is the matrix-v2 break,
// pinned as a test so it cannot recur silently.
func TestRoleBleed(t *testing.T) {
	got, err := compile.Compile(corpus(), builderOnM1())
	if err != nil {
		t.Fatalf("compile builder-on-m1: %v", err)
	}

	forbidden := []struct{ id, why string }{
		{"klaw-orchestrates", "profile:klaw bleeding into builder — the matrix-v2 break"},
		{"m4-disk-tight", "host:m4-mini bleeding onto m1"},
	}
	for _, f := range forbidden {
		if hasFragment(got.Selected, f.id) {
			t.Errorf("role bleed: %q selected for builder-on-m1: %s", f.id, f.why)
		}
		if body := allText(got); strings.Contains(body, textOf(t, f.id)) {
			t.Errorf("role bleed: %q rendered into builder-on-m1 output: %s", f.id, f.why)
		}
	}

	// The inverse half: correct fragments must survive. An invariant that passes by
	// emitting nothing is not an invariant, it is a broken compiler.
	for _, id := range []string{"trash-not-rm", "sessions-yield", "builder-builds"} {
		if !hasFragment(got.Selected, id) {
			t.Errorf("over-filtered: %q must select for builder-on-m1 but did not", id)
		}
	}
}

// TestHarnessBleed asserts the second half of the spec's invariant 1: OpenClaw
// mechanics must not reach a Claude target.
func TestHarnessBleed(t *testing.T) {
	target := compile.Target{
		Name: "claude-global",
		Selector: fragment.Selector{
			Host: "m1", Profile: "builder", Harness: fragment.HarnessClaude,
		},
	}

	got, err := compile.Compile(corpus(), target)
	if err != nil {
		t.Fatalf("compile claude-global: %v", err)
	}

	for _, id := range []string{"sessions-yield", "klaw-orchestrates"} {
		if hasFragment(got.Selected, id) {
			t.Errorf("harness bleed: openclaw fragment %q selected for a claude target", id)
		}
	}
	if !hasFragment(got.Selected, "trash-not-rm") {
		t.Error("over-filtered: harness:any doctrine must reach a claude target")
	}
}

// TestInstanceImmunity asserts spec invariant 3: compile never emits runtime memory,
// however broadly that memory is otherwise scoped.
func TestInstanceImmunity(t *testing.T) {
	got, err := compile.Compile(corpus(), builderOnM1())
	if err != nil {
		t.Fatalf("compile builder-on-m1: %v", err)
	}
	if hasFragment(got.Selected, "wiki-archived") {
		t.Error("instance immunity: a lifecycle:instance fragment was compiled")
	}
}

func hasFragment(fs []fragment.Fragment, id string) bool {
	for _, f := range fs {
		if f.ID == id {
			return true
		}
	}
	return false
}

func allText(r compile.Result) string {
	var b strings.Builder
	for _, content := range r.Files {
		b.WriteString(content)
		b.WriteString("\n")
	}
	return b.String()
}

func textOf(t *testing.T, id string) string {
	t.Helper()
	for _, f := range corpus() {
		if f.ID == id {
			return f.Text
		}
	}
	t.Fatalf("test bug: no fragment %q in corpus", id)
	return ""
}
