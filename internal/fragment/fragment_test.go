package fragment_test

import (
	"testing"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

func frag(host, profile, harness, lifecycle string) fragment.Fragment {
	return fragment.Fragment{
		ID: "f", Text: "t",
		Host: host, Profile: profile, Harness: harness,
		Lifecycle: lifecycle, Kind: fragment.KindRule,
	}
}

// TestSelectsScopeAsymmetry pins the contract that makes role bleed impossible:
// selection flows from "any" down to a concrete target, never sideways between two
// concrete values and never upward from a specific fragment to a broader target.
func TestSelectsScopeAsymmetry(t *testing.T) {
	target := fragment.Selector{Host: "m1", Profile: "builder", Harness: fragment.HarnessOpenClaw}

	cases := []struct {
		name string
		f    fragment.Fragment
		want bool
	}{
		{"universal reaches every target",
			frag(fragment.AxisAny, fragment.AxisAny, fragment.AxisAny, fragment.LifecycleAuthored), true},
		{"exact match on every axis selects",
			frag("m1", "builder", fragment.HarnessOpenClaw, fragment.LifecycleAuthored), true},
		{"other agent's fragment does not bleed in",
			frag(fragment.AxisAny, "klaw", fragment.HarnessOpenClaw, fragment.LifecycleAuthored), false},
		{"other machine's fragment does not bleed in",
			frag("m4-mini", fragment.AxisAny, fragment.AxisAny, fragment.LifecycleAuthored), false},
		{"other harness's fragment does not bleed in",
			frag(fragment.AxisAny, fragment.AxisAny, fragment.HarnessClaude, fragment.LifecycleAuthored), false},
		{"one wrong axis is enough to exclude",
			frag("m1", "builder", fragment.HarnessClaude, fragment.LifecycleAuthored), false},
		{"facts about the human reach every agent",
			frag(fragment.AxisAny, fragment.ProfileUser, fragment.AxisAny, fragment.LifecycleAuthored), true},
		{"runtime memory never compiles, however broadly scoped",
			frag(fragment.AxisAny, fragment.AxisAny, fragment.AxisAny, fragment.LifecycleInstance), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.f.Selects(target); got != tc.want {
				t.Errorf("Selects(%+v) = %v, want %v", target, got, tc.want)
			}
		})
	}
}

// TestSelectsUserProfileIsNotAnAgent guards the ProfileUser exception against the
// obvious wrong reading: "user" must not behave like an agent id that only matches a
// target compiled for an agent literally named "user".
func TestSelectsUserProfileIsNotAnAgent(t *testing.T) {
	f := frag(fragment.AxisAny, fragment.ProfileUser, fragment.AxisAny, fragment.LifecycleAuthored)

	for _, agent := range []string{"klaw", "builder", "researcher", "ops"} {
		s := fragment.Selector{Host: "m1", Profile: agent, Harness: fragment.HarnessOpenClaw}
		if !f.Selects(s) {
			t.Errorf("profile:user must select for agent %q; every agent works for the same human", agent)
		}
	}
}

// TestSelectPreservesOrder pins determinism: same corpus in, same order out. Compile
// output must be byte-stable given identical inputs, or diff reports phantom drift.
func TestSelectPreservesOrder(t *testing.T) {
	corpus := []fragment.Fragment{
		{ID: "a", Text: "a", Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny, Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule},
		{ID: "skip", Text: "s", Host: "m4-mini", Profile: fragment.AxisAny, Harness: fragment.AxisAny, Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact},
		{ID: "b", Text: "b", Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny, Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule},
		{ID: "c", Text: "c", Host: "m1", Profile: fragment.AxisAny, Harness: fragment.AxisAny, Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact},
	}
	target := fragment.Selector{Host: "m1", Profile: "builder", Harness: fragment.HarnessOpenClaw}

	got := fragment.Select(corpus, target)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("Select returned %d fragments, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i].ID != id {
			t.Errorf("Select()[%d].ID = %q, want %q — corpus order must survive selection", i, got[i].ID, id)
		}
	}
}

// TestValidateRejectsUnplaceable: an untagged fragment has no defined home, which is
// the v1 file-as-owner bug wearing a new costume. Reject at the door.
func TestValidateRejectsUnplaceable(t *testing.T) {
	ok := fragment.Fragment{
		ID: "ok", Text: "trash > rm.",
		Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
		Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("fully tagged fragment rejected: %v", err)
	}

	bad := map[string]func(fragment.Fragment) fragment.Fragment{
		"missing id":        func(f fragment.Fragment) fragment.Fragment { f.ID = ""; return f },
		"missing text":      func(f fragment.Fragment) fragment.Fragment { f.Text = " "; return f },
		"missing host":      func(f fragment.Fragment) fragment.Fragment { f.Host = ""; return f },
		"missing profile":   func(f fragment.Fragment) fragment.Fragment { f.Profile = ""; return f },
		"missing harness":   func(f fragment.Fragment) fragment.Fragment { f.Harness = ""; return f },
		"unknown harness":   func(f fragment.Fragment) fragment.Fragment { f.Harness = "cursor"; return f },
		"missing lifecycle": func(f fragment.Fragment) fragment.Fragment { f.Lifecycle = ""; return f },
		"unknown lifecycle": func(f fragment.Fragment) fragment.Fragment { f.Lifecycle = "ephemeral"; return f },
		"missing kind":      func(f fragment.Fragment) fragment.Fragment { f.Kind = ""; return f },
		"unknown kind":      func(f fragment.Fragment) fragment.Fragment { f.Kind = "persona"; return f },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			if err := mutate(ok).Validate(); err == nil {
				t.Errorf("Validate() accepted a fragment with %s", name)
			}
		})
	}
}

// TestValidateAllowsOpenHostAndProfileVocabularies: machine ids and agent ids are
// open sets. Validate must not become a hardcoded roster — that is a config fact,
// looked up, never baked into the tool.
func TestValidateAllowsOpenHostAndProfileVocabularies(t *testing.T) {
	f := fragment.Fragment{
		ID: "f", Text: "t",
		Host: "some-box-bought-next-year", Profile: "some-agent-invented-next-year",
		Harness: fragment.AxisAny, Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact,
	}
	if err := f.Validate(); err != nil {
		t.Errorf("Validate rejected an unknown host/profile id: %v; both are open vocabularies", err)
	}
}
