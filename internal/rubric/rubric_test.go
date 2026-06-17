package rubric

import (
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/config"
)

func TestBuild(t *testing.T) {
	p := &config.Persona{
		Backstory:  "an engineer who shipped at Tesla and OpenAI",
		Voice:      "dry, precise, allergic to filler",
		Opinions:   []string{"The smallest correct change beats the cleverest one."},
		Tensions:   []string{"I prize speed yet stop the line over the unverifiable."},
		Boundaries: []string{"I won't claim code is tested when I haven't verified it."},
		Avoid:      []string{"No corporate hedging."},
		Counters:   []config.Exchange{{Prompt: "x", Response: "Certainly, delighted to help!"}},
	}
	out := Build("coder", p)

	for _, want := range []string{
		"Voice 0–2", "turns-to-flip", "Hedge markers", "Scoring sheet",
		"smallest correct change", // opinion probe
		"stop the line",           // tension probe
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rubric missing %q", want)
		}
	}
	// Specificity markers should pick proper nouns from the backstory.
	if !strings.Contains(out, "Tesla") || !strings.Contains(out, "OpenAI") {
		t.Errorf("expected proper nouns Tesla/OpenAI in signals:\n%s", out)
	}
	// Anti-pattern markers should be derived from Avoid + Counters.
	if !strings.Contains(out, "corporate") {
		t.Errorf("expected anti-pattern marker 'corporate'")
	}
}

func TestBuildEmptyPersona(t *testing.T) {
	out := Build("bare", &config.Persona{})
	if !strings.Contains(out, "No opinions or tensions to probe") {
		t.Errorf("empty persona should note nothing to probe:\n%s", out)
	}
}
