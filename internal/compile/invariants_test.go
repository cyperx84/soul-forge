// The invariants are compile errors, so the only way to know they work is to make
// each one fire. A green invariant test that never fails is decoration.

package compile_test

import (
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/compile"
	"github.com/cyperx84/soul-forge/internal/fragment"
)

func base(text string) fragment.Fragment {
	return fragment.Fragment{
		ID: "f", Text: text,
		Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
		Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
	}
}

func openclawTarget() compile.Target {
	return compile.Target{
		Name:     "openclaw-hub",
		Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessOpenClaw},
	}
}

// TestInvariantsFire: each one must stop the build on the thing it exists to stop.
func TestInvariantsFire(t *testing.T) {
	cases := []struct {
		name      string
		f         fragment.Fragment
		invariant string
	}{
		{
			name: "hardcoded model name",
			f: func() fragment.Fragment {
				f := base("Delegate coding to claude-opus-4-8.")
				return f
			}(),
			invariant: "no-hardcoded-models",
		},
		{
			name:      "anthropic api key",
			f:         base("Auth with sk-ant-api03-EXAMPLE-NOT-A-REAL-KEY."),
			invariant: "no-secrets",
		},
		{
			name:      "github token",
			f:         base("Use ghp_EXAMPLENOTAREALTOKEN for the push."),
			invariant: "no-secrets",
		},
		{
			name:      "machine path tagged host:any",
			f:         base("Workspace lives at /Users/cyperx/.openclaw/workspace."),
			invariant: "untagged-machine-fact",
		},
		{
			name:      "duplicates runtime-injected NO_REPLY",
			f:         base("If no response is needed, reply NO_REPLY."),
			invariant: "runtime-non-duplication",
		},
		{
			name: "instance fragment reaching compile",
			f: func() fragment.Fragment {
				f := base("The fleet wiki got archived.")
				f.Lifecycle = fragment.LifecycleInstance
				return f
			}(),
			// Selection already excludes instance fragments, so this asserts the
			// belt-and-braces path: it must not compile even if selection let it by.
			invariant: "",
		},
		{
			name: "subagent-needed rule rendering outside AGENTS/TOOLS",
			f: func() fragment.Fragment {
				f := base("Never run tmux kill-server.")
				f.Kind = fragment.KindVoice // routes to SOUL.md, which sub-agents never see
				f.NeededBy = []string{fragment.NeededBySubagent}
				return f
			}(),
			invariant: "subagent-self-sufficiency",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compile.Compile([]fragment.Fragment{tc.f}, openclawTarget())

			if tc.invariant == "" {
				// Instance immunity: excluded at selection, so compile succeeds but
				// must emit nothing.
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(got.Selected) != 0 {
					t.Errorf("instance fragment survived selection: %+v", got.Selected)
				}
				for path, body := range got.Files {
					if strings.Contains(body, tc.f.Text) {
						t.Errorf("instance fragment rendered into %s", path)
					}
				}
				return
			}

			if err == nil {
				t.Fatalf("invariant %q did not fire; compile succeeded with files %v", tc.invariant, got.Files)
			}
			var ie *compile.InvariantError
			if !asInvariant(err, &ie) {
				t.Fatalf("want InvariantError, got %T: %v", err, err)
			}
			if ie.Invariant != tc.invariant {
				t.Errorf("fired invariant %q, want %q (err: %v)", ie.Invariant, tc.invariant, err)
			}
		})
	}
}

// TestInvariantsDoNotFireOnCleanFragments guards the other side: an invariant that
// fires on correct input is worse than none, because it trains people to bypass it.
func TestInvariantsDoNotFireOnCleanFragments(t *testing.T) {
	clean := []fragment.Fragment{
		base("trash > rm."),
		base("Act first, explain after."),
		{
			ID: "host-fact", Text: "Workspace lives at /Users/cyperx/.openclaw/workspace.",
			Host: "m4-mini", Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact,
		},
		{
			ID: "subagent-rule", Text: "Never run tmux kill-server.",
			Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
			NeededBy: []string{fragment.NeededBySubagent},
		},
		{
			ID: "model-lookup", Text: "Model names live in config — look them up, never hardcode.",
			Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
		},
	}
	// IDs must be unique or the duplicate check fires instead.
	for i := range clean {
		if clean[i].ID == "f" {
			clean[i].ID = "clean-" + string(rune('a'+i))
		}
	}

	if _, err := compile.Compile(clean, openclawTarget()); err != nil {
		t.Fatalf("invariant fired on clean fragments: %v", err)
	}
}

// TestHostTaggedMachineFactIsAllowed: the untagged-machine-fact invariant must key on
// the missing tag, not on the path. A correctly tagged machine fact is the whole
// point of the host axis.
func TestHostTaggedMachineFactIsAllowed(t *testing.T) {
	f := fragment.Fragment{
		ID: "hermes-path", Text: "Hermes lives at /Users/cyperx/.hermes.",
		Host: "m4-mini", Profile: fragment.AxisAny, Harness: fragment.AxisAny,
		Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact,
	}
	got, err := compile.Compile([]fragment.Fragment{f}, openclawTarget())
	if err != nil {
		t.Fatalf("host-tagged machine fact rejected: %v", err)
	}
	if !strings.Contains(got.Files["TOOLS.md"], "/Users/cyperx/.hermes") {
		t.Errorf("host-tagged fact must render into TOOLS.md; got files %v", keys(got.Files))
	}
}

// TestDuplicateFragmentIDsFail: two definitions of one ID have no defined precedence,
// so the build must stop rather than pick one.
func TestDuplicateFragmentIDsFail(t *testing.T) {
	a := base("First definition.")
	b := base("Second definition.")
	_, err := compile.Compile([]fragment.Fragment{a, b}, openclawTarget())
	if err == nil {
		t.Fatal("duplicate fragment ids compiled without error")
	}
	if !strings.Contains(err.Error(), "duplicate fragment ids") {
		t.Errorf("unclear error for duplicate ids: %v", err)
	}
}

// TestBudgetFailsBuild: workspace files inject every session, so bloat is a
// per-session tax, not a style problem.
func TestBudgetFailsBuild(t *testing.T) {
	f := base(strings.Repeat("x", 500))
	target := openclawTarget()
	target.MaxBytesPerFile = 100

	if _, err := compile.Compile([]fragment.Fragment{f}, target); err == nil {
		t.Fatal("over-budget file compiled without error")
	}
}

func asInvariant(err error, target **compile.InvariantError) bool {
	ie, ok := err.(*compile.InvariantError)
	if ok {
		*target = ie
	}
	return ok
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
