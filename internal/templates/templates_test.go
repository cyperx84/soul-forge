package templates

import (
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/config"
	"github.com/cyperx84/soul-forge/internal/profile"
)

func TestRenderSoul(t *testing.T) {
	data := TemplateData{
		AgentName: "alpha",
		AgentRole: "builder",
		Channel:   "ops",
		Persona: &config.Persona{
			Backstory:  "a careful builder",
			Voice:      "terse",
			Opinions:   []string{"simple beats clever"},
			Principles: []string{"read before writing"},
			Boundaries: []string{"never delete unasked"},
			Avoid:      []string{"no filler"},
			Examples:   []config.Exchange{{Prompt: "ping?", Response: "pong."}},
		},
		Profile: &profile.Profile{
			Identity: profile.Identity{Name: "Chris", TechnicalSkill: "expert", ExpertiseAreas: []string{"Go"}},
			WorkStyle: profile.WorkStyle{
				Preferences:       []string{"speed"},
				OutputPreferences: map[string]string{"b": "2", "a": "1"},
			},
		},
	}
	out, err := Render("soul.md.tmpl", data)
	if err != nil {
		t.Fatal(err)
	}
	// First-person identity, persona content, calibration, sorted output prefs, example.
	for _, s := range []string{
		"I am **alpha**", "a careful builder", "simple beats clever",
		"read before writing", "never delete unasked", "no filler",
		"Working With Chris", "expert", "ping?", "pong.",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("soul.md missing %q\n---\n%s", s, out)
		}
	}
	// OutputPreferences must be deterministically sorted (a before b).
	if strings.Index(out, "**a:** 1") > strings.Index(out, "**b:** 2") {
		t.Errorf("output preferences not sorted:\n%s", out)
	}
	// No legacy placeholder noise.
	if strings.Contains(out, "Not specified") {
		t.Errorf("soul.md should not contain placeholder 'Not specified':\n%s", out)
	}
}

func TestRenderGracefulWhenEmpty(t *testing.T) {
	// Empty profile + persona: SOUL still renders an identity line, USER omits empty sections.
	data := TemplateData{AgentName: "solo", AgentRole: "general", Profile: &profile.Profile{}}

	soul, err := Render("soul.md.tmpl", data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(soul, "I am **solo**") {
		t.Errorf("soul.md missing identity line:\n%s", soul)
	}
	if strings.Contains(soul, "## Working With") {
		t.Errorf("soul.md should omit empty 'Working With' section:\n%s", soul)
	}

	// Name-only profile: the name appears only in the heading, so there's no body —
	// the section must still be omitted (regression guard for an audit false-positive).
	nameOnly := TemplateData{AgentName: "solo", AgentRole: "general",
		Profile: &profile.Profile{Identity: profile.Identity{Name: "Sam"}}}
	soul2, err := Render("soul.md.tmpl", nameOnly)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(soul2, "## Working With") {
		t.Errorf("name-only profile should omit 'Working With' section:\n%s", soul2)
	}

	usr, err := Render("user.md.tmpl", data)
	if err != nil {
		t.Fatal(err)
	}
	for _, heading := range []string{"## Identity", "## Work Style", "## Environment"} {
		if strings.Contains(usr, heading) {
			t.Errorf("user.md should omit empty section %q:\n%s", heading, usr)
		}
	}
	if strings.Contains(usr, "Not specified") {
		t.Errorf("user.md should not contain 'Not specified':\n%s", usr)
	}
}

func TestRenderAllTemplates(t *testing.T) {
	data := TemplateData{
		AgentName: "ace", AgentRole: "coding",
		Persona:   &config.Persona{Voice: "x", Vibe: "sharp", Emoji: "🔧"},
		Operating: []string{"verify before acting"},
		Profile: &profile.Profile{
			Environment: profile.Environment{OS: "macOS", KeyTools: []string{"fzf"}},
			WorkStyle:   profile.WorkStyle{Languages: []string{"Go"}},
		},
	}
	// SOUL/IDENTITY/AGENTS/TOOLS/MEMORY are keyed to the agent; USER.md is keyed to the human.
	for _, name := range []string{"soul.md.tmpl", "identity.md.tmpl", "agents.md.tmpl", "tools.md.tmpl", "memory.md.tmpl"} {
		out, err := Render(name, data)
		if err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		if !strings.Contains(out, "ace") {
			t.Errorf("%s missing agent name:\n%s", name, out)
		}
	}
	if out, err := Render("user.md.tmpl", data); err != nil || !strings.Contains(out, "USER.md") {
		t.Errorf("user.md render failed: err=%v\n%s", err, out)
	}

	// AGENTS.md renders the operating rules as a numbered list.
	if out, err := Render("agents.md.tmpl", data); err != nil || !strings.Contains(out, "1. verify before acting") {
		t.Errorf("agents.md missing numbered operating rule: err=%v\n%s", err, out)
	}
	// IDENTITY.md carries the vibe/emoji.
	if out, err := Render("identity.md.tmpl", data); err != nil || !strings.Contains(out, "🔧") || !strings.Contains(out, "sharp") {
		t.Errorf("identity.md missing vibe/emoji: err=%v\n%s", err, out)
	}
}

func TestRenderMissingTemplate(t *testing.T) {
	if _, err := Render("missing.tmpl", TemplateData{Profile: &profile.Profile{}}); err == nil {
		t.Fatal("expected missing template error")
	}
}

func TestUserName(t *testing.T) {
	if got := (TemplateData{Profile: &profile.Profile{}}).UserName(); got != "this user" {
		t.Errorf("UserName() empty = %q, want 'this user'", got)
	}
	d := TemplateData{Profile: &profile.Profile{Identity: profile.Identity{Name: "Ada"}}}
	if got := d.UserName(); got != "Ada" {
		t.Errorf("UserName() = %q, want 'Ada'", got)
	}
}
