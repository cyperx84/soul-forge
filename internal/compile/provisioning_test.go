// The acceptance case, end to end: one corpus, two agents, two machines, two
// harnesses. This is the scenario the hand-built matrix could not survive — Builder
// inheriting "Klaw orchestrates" — exercised through the real path rather than
// asserted in the abstract.

package compile_test

import (
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/compile"
	"github.com/cyperx84/soul-forge/internal/fragment"
)

// fleet builds the prime -> machine -> agent chain the M1 provisioning plan needs.
func fleet(machine, agent string) *fragment.Corpus {
	prime := &fragment.Corpus{
		Name: "prime",
		Fragments: []fragment.Fragment{
			{ID: "trash-not-rm", Text: "trash > rm.",
				Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
				Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
				NeededBy: []string{fragment.NeededBySubagent}},
			{ID: "secrets-never-git", Text: "Credentials never enter git, config, or any tracked file.",
				Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
				Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule},
			{ID: "chris-pushback", Text: "Wants real opinions and pushback, not validation.",
				Host: fragment.AxisAny, Profile: fragment.ProfileUser, Harness: fragment.AxisAny,
				Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact},
			{ID: "sessions-yield", Text: "Wait with sessions_yield — never busy-poll.",
				Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.HarnessOpenClaw,
				Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule},
		},
	}

	machines := map[string]fragment.Fragment{
		"m4-mini": {ID: "m4-disk", Text: "Disk chronically tight (228GB, often 90%+).",
			Host: "m4-mini", Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact},
		"m1": {ID: "m1-clean", Text: "Clean box; GPG keys are born here.",
			Host: "m1", Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact},
	}

	agents := map[string]fragment.Fragment{
		"klaw": {ID: "klaw-orchestrates", Text: "Klaw orchestrates the fleet.",
			Host: fragment.AxisAny, Profile: "klaw", Harness: fragment.HarnessOpenClaw,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindIdentity},
		"builder": {ID: "builder-builds", Text: "Builder implements; it does not route work.",
			Host: fragment.AxisAny, Profile: "builder", Harness: fragment.HarnessOpenClaw,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindIdentity},
	}

	machineCorpus := &fragment.Corpus{Name: machine, Extends: prime, Fragments: []fragment.Fragment{machines[machine]}}
	return &fragment.Corpus{Name: agent, Extends: machineCorpus, Fragments: []fragment.Fragment{agents[agent]}}
}

func resolve(t *testing.T, c *fragment.Corpus) []fragment.Fragment {
	t.Helper()
	got, _, err := c.Resolve()
	if err != nil {
		t.Fatalf("Resolve %s: %v", c.Name, err)
	}
	return got
}

// TestProvisioningBuilderOnM1IsCleanOfKlawAndM4 is the acceptance test for the whole
// model: derive Builder on the M1 prime box from the same corpus that produces Klaw
// on the M4, and assert neither the other agent's identity nor the other machine's
// facts follow it across.
func TestProvisioningBuilderOnM1IsCleanOfKlawAndM4(t *testing.T) {
	corpus := resolve(t, fleet("m1", "builder"))
	got, err := compile.Compile(corpus, compile.Target{
		Name:     "openclaw-worker",
		Selector: fragment.Selector{Host: "m1", Profile: "builder", Harness: fragment.HarnessOpenClaw},
	})
	if err != nil {
		t.Fatalf("compile builder-on-m1: %v", err)
	}

	body := strings.Join(values(got.Files), "\n")
	for _, leak := range []string{"Klaw orchestrates", "228GB"} {
		if strings.Contains(body, leak) {
			t.Errorf("builder-on-m1 output contains %q — this is the matrix-v2 break", leak)
		}
	}
	for _, want := range []string{"Builder implements", "GPG keys are born here", "trash > rm"} {
		if !strings.Contains(body, want) {
			t.Errorf("builder-on-m1 output missing %q", want)
		}
	}
	if id := got.Files["IDENTITY.md"]; !strings.Contains(id, "Builder implements") {
		t.Errorf("IDENTITY.md = %q, want the agent's role card", id)
	}
	if tools := got.Files["TOOLS.md"]; !strings.Contains(tools, "GPG keys are born here") {
		t.Errorf("TOOLS.md = %q, want this machine's facts", tools)
	}
}

// TestProvisioningKlawOnM4KeepsItsOwn: the same corpus, the other agent. Guards
// against a compiler that passes the bleed test by emitting too little.
func TestProvisioningKlawOnM4KeepsItsOwn(t *testing.T) {
	corpus := resolve(t, fleet("m4-mini", "klaw"))
	got, err := compile.Compile(corpus, compile.Target{
		Name:     "openclaw-hub",
		Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessOpenClaw},
	})
	if err != nil {
		t.Fatalf("compile klaw-on-m4: %v", err)
	}

	body := strings.Join(values(got.Files), "\n")
	for _, want := range []string{"Klaw orchestrates", "228GB", "trash > rm", "Wants real opinions"} {
		if !strings.Contains(body, want) {
			t.Errorf("klaw-on-m4 output missing %q", want)
		}
	}
	if strings.Contains(body, "Builder implements") {
		t.Error("klaw-on-m4 output contains builder's identity")
	}
	if u := got.Files["USER.md"]; !strings.Contains(u, "Wants real opinions") {
		t.Errorf("USER.md = %q, want facts about the human", u)
	}
}

// TestSubagentSelfSufficiency: a rule a delegated worker needs must land in AGENTS.md
// or TOOLS.md, the only two files a sub-agent session is injected with
// (concepts/system-prompt.md:227).
func TestSubagentSelfSufficiency(t *testing.T) {
	corpus := resolve(t, fleet("m4-mini", "klaw"))
	got, err := compile.Compile(corpus, compile.Target{
		Name:     "openclaw-hub",
		Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessOpenClaw},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(got.Files["AGENTS.md"], "trash > rm") {
		t.Error("a needed_by:subagent rule must reach AGENTS.md, or delegated workers never see it")
	}
}

// TestOneFragmentTwoHarnesses is the payoff: the M4 disk fact is authored once and
// reaches both OpenClaw's TOOLS.md and Claude Code's CLAUDE.md. Rendered duplication
// is correct; hand-owned duplication is the bug this replaces.
func TestOneFragmentTwoHarnesses(t *testing.T) {
	corpus := resolve(t, fleet("m4-mini", "klaw"))

	openclaw, err := compile.Compile(corpus, compile.Target{
		Name:     "openclaw-hub",
		Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessOpenClaw},
	})
	if err != nil {
		t.Fatalf("compile openclaw: %v", err)
	}
	claude, err := compile.Compile(corpus, compile.Target{
		Name:     "claude-global",
		Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessClaude},
	})
	if err != nil {
		t.Fatalf("compile claude: %v", err)
	}

	if !strings.Contains(openclaw.Files["TOOLS.md"], "228GB") {
		t.Error("machine fact missing from OpenClaw TOOLS.md")
	}
	if !strings.Contains(claude.Files["CLAUDE.md"], "228GB") {
		t.Error("machine fact missing from Claude CLAUDE.md — one fragment must reach both harnesses")
	}
	// And the harness-specific rule must not cross over.
	if strings.Contains(claude.Files["CLAUDE.md"], "sessions_yield") {
		t.Error("OpenClaw mechanics leaked into CLAUDE.md")
	}
}

// TestClaudeTargetTakesNoVoice: Claude Code is a tool, not a character. The drop is a
// design position, so it must be reported rather than silent.
func TestClaudeTargetTakesNoVoice(t *testing.T) {
	corpus := []fragment.Fragment{
		{ID: "voice-dense", Text: "Dense over verbose: short beats long.",
			Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindVoice},
		{ID: "rule-trash", Text: "trash > rm.",
			Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule},
	}
	got, err := compile.Compile(corpus, compile.Target{
		Name:     "claude-global",
		Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessClaude},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if strings.Contains(got.Files["CLAUDE.md"], "Dense over verbose") {
		t.Error("voice fragment rendered into a Claude target; Claude Code gets no persona")
	}
	if len(got.Dropped) != 1 || got.Dropped[0].ID != "voice-dense" {
		t.Errorf("Dropped = %+v, want the voice fragment reported — silent drops build surprises", got.Dropped)
	}
	if !strings.Contains(got.Files["CLAUDE.md"], "trash > rm") {
		t.Error("rule missing from CLAUDE.md")
	}
}

// TestCompileIsDeterministic: diff compares compiled output against disk, so any
// nondeterminism would report drift that isn't there and train the reader to ignore
// the alarm.
func TestCompileIsDeterministic(t *testing.T) {
	corpus := resolve(t, fleet("m4-mini", "klaw"))
	target := compile.Target{
		Name:     "openclaw-hub",
		Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessOpenClaw},
	}

	first, err := compile.Compile(corpus, target)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for i := 0; i < 50; i++ {
		again, err := compile.Compile(corpus, target)
		if err != nil {
			t.Fatalf("compile run %d: %v", i, err)
		}
		if len(again.Files) != len(first.Files) {
			t.Fatalf("run %d produced %d files, first produced %d", i, len(again.Files), len(first.Files))
		}
		for path, body := range first.Files {
			if again.Files[path] != body {
				t.Fatalf("run %d: %s differs from first run — compile must be byte-stable", i, path)
			}
		}
	}
}

// TestMemorySkeletonIsNeverFilled: MEMORY.md holds runtime memory. Compile owns its
// existence, never its contents.
func TestMemorySkeletonIsNeverFilled(t *testing.T) {
	corpus := append(resolve(t, fleet("m4-mini", "klaw")), fragment.Fragment{
		ID: "runtime-note", Text: "The fleet wiki got archived.",
		Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
		Lifecycle: fragment.LifecycleInstance, Kind: fragment.KindFact,
	})
	got, err := compile.Compile(corpus, compile.Target{
		Name:     "openclaw-hub",
		Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessOpenClaw},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	mem, ok := got.Files["MEMORY.md"]
	if !ok {
		t.Fatal("MEMORY.md skeleton not created; compile owns the file's existence")
	}
	if strings.Contains(mem, "fleet wiki") {
		t.Error("runtime memory compiled into MEMORY.md")
	}
}

func values(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}
