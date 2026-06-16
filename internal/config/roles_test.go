package config

import (
	"strings"
	"testing"
)

func TestCanonicalRole(t *testing.T) {
	cases := map[string]string{
		"coding": "coding", "software-engineer": "coding", "SWE": "coding",
		"devops": "infrastructure", "sre": "infrastructure",
		"orchestrator": "orchestrator", "coordinator": "orchestrator",
		"researcher": "research",
		"":           "general", "something-else": "general",
	}
	for in, want := range cases {
		if got := CanonicalRole(in); got != want {
			t.Errorf("CanonicalRole(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultPersonaIsSharp(t *testing.T) {
	for _, role := range []string{"coding", "infrastructure", "orchestrator", "research", "general"} {
		p := DefaultPersona(role)
		if p == nil || p.Voice == "" || len(p.Opinions) == 0 || len(p.Boundaries) == 0 {
			t.Fatalf("role %q has a thin default persona: %+v", role, p)
		}
		// Defaults must not contain the hedging phrases the audit linter rejects.
		blob := strings.ToLower(p.Voice + " " + strings.Join(p.Opinions, " ") +
			" " + strings.Join(p.Principles, " ") + " " + strings.Join(p.Boundaries, " ") +
			" " + strings.Join(p.Avoid, " "))
		for _, vague := range []string{"be helpful", "maintain professionalism", "as an ai", "comprehensive and thoughtful"} {
			if strings.Contains(blob, vague) {
				t.Errorf("role %q default persona contains vague phrase %q", role, vague)
			}
		}
	}
}

func TestDefaultOperatingRules(t *testing.T) {
	for _, role := range []string{"coding", "infrastructure", "orchestrator", "research", "general"} {
		if len(DefaultOperatingRules(role)) == 0 {
			t.Errorf("role %q has no operating rules", role)
		}
	}
	// Operating rules are action rules, kept out of the persona's stance fields.
	persona := DefaultPersona("coding")
	ops := strings.Join(DefaultOperatingRules("coding"), " ")
	for _, opinion := range persona.Opinions {
		if strings.Contains(ops, opinion) {
			t.Errorf("opinion leaked into operating rules: %q", opinion)
		}
	}
}

func TestEffectivePersonaMerge(t *testing.T) {
	// No author persona → pure role defaults.
	base := Agent{Name: "a", Role: "coding"}.EffectivePersona()
	if base.Backstory == "" {
		t.Fatal("expected role-default backstory")
	}

	// Author scalars override; author list entries append to defaults, deduped.
	a := Agent{
		Name: "a", Role: "coding",
		Persona: &Persona{
			Voice:    "custom voice",
			Opinions: []string{"I'd rather delete code than add it.", "Tests are part of the change, not a follow-up."}, // 2nd is a dup of a default
		},
	}
	eff := a.EffectivePersona()
	if eff.Voice != "custom voice" {
		t.Errorf("author voice should win, got %q", eff.Voice)
	}
	if eff.Backstory != base.Backstory {
		t.Errorf("backstory should fall back to default")
	}
	// Custom opinion appended; duplicate not doubled.
	var deletes, tests int
	for _, o := range eff.Opinions {
		if o == "I'd rather delete code than add it." {
			deletes++
		}
		if o == "Tests are part of the change, not a follow-up." {
			tests++
		}
	}
	if deletes != 1 {
		t.Errorf("custom opinion count = %d, want 1", deletes)
	}
	if tests != 1 {
		t.Errorf("duplicate opinion was not deduped (count=%d)", tests)
	}
}

func TestPersonaHasContent(t *testing.T) {
	if (&Persona{}).HasContent() {
		t.Error("empty persona should have no content")
	}
	if !(&Persona{Voice: "x"}).HasContent() {
		t.Error("persona with voice should have content")
	}
	var nilP *Persona
	if nilP.HasContent() {
		t.Error("nil persona should have no content")
	}
}
