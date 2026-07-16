package compile

import (
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

// Hermes and codex render maps close the harness set: every valid harness value
// now compiles without Target.Render. The contracts pinned here are the round-8
// findings that produced them.

func hermesTarget() Target {
	return Target{
		Name:     "hermes-home",
		Selector: fragment.Selector{Host: "box-a", Profile: "scout", Harness: fragment.HarnessHermes},
	}
}

func TestHermesSoulCarriesDoctrineAndVoice(t *testing.T) {
	corpus := []fragment.Fragment{
		{ID: "rule", Text: "Never delete without a backup.", Host: fragment.AxisAny, Profile: fragment.AxisAny,
			Harness: fragment.AxisAny, Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule},
		{ID: "voice", Text: "Speak tersely.", Host: fragment.AxisAny, Profile: "scout",
			Harness: fragment.AxisAny, Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindVoice},
	}
	res, err := Compile(corpus, hermesTarget())
	if err != nil {
		t.Fatal(err)
	}
	soul, ok := res.Files["SOUL.md"]
	if !ok {
		t.Fatal("hermes target did not render SOUL.md")
	}
	// Hermes has no home-level AGENTS.md, so SOUL.md is the only authored slot:
	// a rule routed anywhere else never reaches the harness.
	if !strings.Contains(soul, "Never delete without a backup.") {
		t.Error("doctrine missing from hermes SOUL.md — a rule has no other home in this harness")
	}
	if !strings.Contains(soul, "Speak tersely.") {
		t.Error("voice missing from hermes SOUL.md")
	}
}

func TestHermesNeverEmitsUserMD(t *testing.T) {
	corpus := []fragment.Fragment{
		{ID: "who", Text: "The user prefers terse answers.", Host: fragment.AxisAny, Profile: fragment.ProfileUser,
			Harness: fragment.AxisAny, Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact},
	}
	res, err := Compile(corpus, hermesTarget())
	if err != nil {
		t.Fatal(err)
	}
	// Hermes' memory tool writes USER.md at runtime. A compile that emitted it
	// would clobber real memory — the spec bug fixed in 3c02870, pinned.
	if _, exists := res.Files["USER.md"]; exists {
		t.Fatal("hermes target emitted USER.md — that path is runtime-owned memory")
	}
}

func TestCodexDropsVoice(t *testing.T) {
	corpus := []fragment.Fragment{
		{ID: "rule", Text: "Pin dependencies.", Host: fragment.AxisAny, Profile: fragment.AxisAny,
			Harness: fragment.AxisAny, Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule},
		{ID: "voice", Text: "Be warm.", Host: fragment.AxisAny, Profile: fragment.AxisAny,
			Harness: fragment.AxisAny, Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindVoice},
	}
	target := Target{
		Name:     "codex-global",
		Selector: fragment.Selector{Host: "box-a", Profile: fragment.AxisAny, Harness: fragment.HarnessCodex},
	}
	res, err := Compile(corpus, target)
	if err != nil {
		t.Fatal(err)
	}
	agents := res.Files["AGENTS.md"]
	if !strings.Contains(agents, "Pin dependencies.") {
		t.Error("rule missing from codex AGENTS.md")
	}
	if strings.Contains(agents, "Be warm.") {
		t.Error("codex is a tool, not a character — voice must be dropped at the map")
	}
}
