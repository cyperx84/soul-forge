package fragment_test

import (
	"testing"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

func rule(id, text string) fragment.Fragment {
	return fragment.Fragment{
		ID: id, Text: text,
		Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
		Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
	}
}

// prime -> machine -> agent is the provisioning chain: prime (clean box, GPG keys
// born there) holds universal doctrine, the machine adds its own facts, the agent
// adds its role.
func provisioningChain() *fragment.Corpus {
	prime := &fragment.Corpus{
		Name:      "prime",
		Fragments: []fragment.Fragment{rule("trash-not-rm", "trash > rm."), rule("act-first", "Act first, explain after.")},
	}
	machine := &fragment.Corpus{
		Name:    "m4-mini",
		Extends: prime,
		Fragments: []fragment.Fragment{{
			ID: "disk-tight", Text: "Disk chronically tight.",
			Host: "m4-mini", Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact,
		}},
	}
	return &fragment.Corpus{
		Name:    "klaw",
		Extends: machine,
		Fragments: []fragment.Fragment{{
			ID: "klaw-orchestrates", Text: "Klaw orchestrates the fleet.",
			Host: fragment.AxisAny, Profile: "klaw", Harness: fragment.HarnessOpenClaw,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindIdentity,
		}},
	}
}

// TestResolveInheritsParentFirst: a reader of compiled output should meet inherited
// doctrine before local additions, and resolution must be deterministic.
func TestResolveInheritsParentFirst(t *testing.T) {
	got, overrides, err := provisioningChain().Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(overrides) != 0 {
		t.Errorf("unexpected overrides: %+v", overrides)
	}

	want := []string{"trash-not-rm", "act-first", "disk-tight", "klaw-orchestrates"}
	if len(got) != len(want) {
		t.Fatalf("Resolve returned %d fragments, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("Resolve()[%d].ID = %q, want %q — chain order must be root-first", i, got[i].ID, id)
		}
	}
}

// TestResolveNeverSilentlyDropsParentFragments is the red line of the whole
// inheritance model: a child may add or override, but a parent's rule must not vanish
// on a downstream box. A silent drop is how a red line disappears without anyone
// noticing.
func TestResolveNeverSilentlyDropsParentFragments(t *testing.T) {
	chain := provisioningChain()
	parentIDs := map[string]bool{}
	for c := chain; c != nil; c = c.Extends {
		for _, f := range c.Fragments {
			parentIDs[f.ID] = true
		}
	}

	got, _, err := chain.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	for id := range parentIDs {
		found := false
		for _, f := range got {
			if f.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("fragment %q from the inheritance chain was dropped by Resolve", id)
		}
	}
}

// TestResolveOverrideKeepsPositionAndIsReported: an override is a redefinition, not a
// re-prioritisation, so it holds the parent's slot. And it must be reported — a
// downstream box quietly changing its parent's doctrine is exactly the drift this
// tool exists to surface.
func TestResolveOverrideKeepsPositionAndIsReported(t *testing.T) {
	prime := &fragment.Corpus{
		Name:      "prime",
		Fragments: []fragment.Fragment{rule("a", "first"), rule("install", "Homebrew first."), rule("z", "last")},
	}
	child := &fragment.Corpus{
		Name:      "m1",
		Extends:   prime,
		Fragments: []fragment.Fragment{rule("install", "Nix first.")},
	}

	got, overrides, err := child.Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("override changed corpus size to %d, want 3 — an override replaces, it does not append", len(got))
	}
	if got[1].ID != "install" || got[1].Text != "Nix first." {
		t.Errorf("got[1] = %+v; override must take the parent's position with the child's content", got[1])
	}

	if len(overrides) != 1 {
		t.Fatalf("Resolve reported %d overrides, want 1 — silent overrides are drift", len(overrides))
	}
	o := overrides[0]
	if o.ID != "install" || o.ChildProfile != "m1" || o.Parent.Text != "Homebrew first." || o.Child.Text != "Nix first." {
		t.Errorf("override report is wrong: %+v", o)
	}
}

// TestResolveDetectsCycle: an extends-loop is easy to create by hand when cloning
// profiles, and it would otherwise hang the compiler.
func TestResolveDetectsCycle(t *testing.T) {
	a := &fragment.Corpus{Name: "a", Fragments: []fragment.Fragment{rule("x", "x")}}
	b := &fragment.Corpus{Name: "b", Extends: a, Fragments: []fragment.Fragment{rule("y", "y")}}
	a.Extends = b // the loop

	if _, _, err := b.Resolve(); err == nil {
		t.Fatal("Resolve accepted an extends cycle")
	}
}

// TestResolveRejectsInvalidFragment: validation at resolve, so a bad tag fails at the
// profile that introduced it and names that profile.
func TestResolveRejectsInvalidFragment(t *testing.T) {
	bad := rule("bad", "text")
	bad.Kind = "persona" // v1's model; deleted in v2
	c := &fragment.Corpus{Name: "sneaky", Fragments: []fragment.Fragment{bad}}

	_, _, err := c.Resolve()
	if err == nil {
		t.Fatal("Resolve accepted a fragment with an invalid kind")
	}
	if want := "sneaky"; !contains(err.Error(), want) {
		t.Errorf("error %q should name the profile %q that introduced the bad fragment", err, want)
	}
}

// TestResolveNilCorpus: cloning code walks Extends chains, so nil must be boring.
func TestResolveNilCorpus(t *testing.T) {
	var c *fragment.Corpus
	got, overrides, err := c.Resolve()
	if err != nil || got != nil || overrides != nil {
		t.Errorf("nil corpus Resolve() = (%v, %v, %v), want all nil", got, overrides, err)
	}
}

func TestDuplicateIDs(t *testing.T) {
	fs := []fragment.Fragment{rule("a", "1"), rule("b", "2"), rule("a", "3"), rule("c", "4"), rule("c", "5")}
	got := fragment.DuplicateIDs(fs)
	want := []string{"a", "c"}
	if len(got) != len(want) {
		t.Fatalf("DuplicateIDs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("DuplicateIDs()[%d] = %q, want %q (result must be sorted for determinism)", i, got[i], want[i])
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
