package templates

import (
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/profile"
)

func TestRenderAndHelpers(t *testing.T) {
	data := TemplateData{
		AgentName: "alpha",
		AgentRole: "builder",
		Channel:   "ops",
		Profile: &profile.Profile{
			Identity: profile.Identity{Name: "Chris", ExpertiseAreas: []string{"Go"}},
			WorkStyle: profile.WorkStyle{
				Preferences:       []string{"speed"},
				OutputPreferences: map[string]string{"b": "2", "a": "1"},
				Languages:         []string{"Go"},
				Tools:             []string{"git"},
			},
			Environment: profile.Environment{OS: "macOS", Shell: "zsh", Editor: "nvim"},
		},
	}
	out, err := Render("soul.md.tmpl", data)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"You are **alpha**", "**a:** 1", "**b:** 2", "macOS", "Go"} {
		if !strings.Contains(out, s) {
			t.Fatalf("missing %q in %s", s, out)
		}
	}
	out, err = Render("user.md.tmpl", TemplateData{AgentName: "alpha", AgentRole: "builder", Profile: &profile.Profile{}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Not specified") {
		t.Fatalf("expected defaults in %s", out)
	}
	if _, err := Render("missing.tmpl", data); err == nil {
		t.Fatal("expected missing template error")
	}
}

func TestRoleAwareTemplates(t *testing.T) {
	roles := []struct {
		role        string
		soulMarker  string
		userMarker  string
	}{
		{"coding", "Technical Stack", "Technical Profile"},
		{"infrastructure", "System Context", "System Environment"},
		{"research", "Research Focus", "Research Profile"},
		{"orchestrator", "Coordinate, don't micromanage", "Current Goals"},
		{"general", "Behavioral Guidelines", "Not specified"},
	}

	prof := &profile.Profile{
		Identity: profile.Identity{
			Name:           "Chris",
			Goals:          []string{"ship it"},
			ExpertiseAreas: []string{"Go"},
			LearningFocus:  []string{"Rust"},
		},
		WorkStyle: profile.WorkStyle{
			Tools:     []string{"git"},
			Languages: []string{"Go"},
			DoNotDo:   []string{"no mocks"},
		},
		Environment: profile.Environment{OS: "macOS", Shell: "zsh", Editor: "nvim"},
	}

	for _, tc := range roles {
		t.Run(tc.role, func(t *testing.T) {
			data := TemplateData{AgentName: "forge", AgentRole: tc.role, Profile: prof}

			soul, err := Render("soul.md.tmpl", data)
			if err != nil {
				t.Fatalf("soul render: %v", err)
			}
			if !strings.Contains(soul, "You are **forge**") {
				t.Errorf("missing agent name in soul.md for role %s", tc.role)
			}
			if !strings.Contains(soul, tc.soulMarker) {
				t.Errorf("missing role-specific marker %q in soul.md for role %s\ngot:\n%s", tc.soulMarker, tc.role, soul)
			}

			usr, err := Render("user.md.tmpl", data)
			if err != nil {
				t.Fatalf("user render: %v", err)
			}
			if !strings.Contains(usr, tc.userMarker) {
				t.Errorf("missing role-specific marker %q in user.md for role %s\ngot:\n%s", tc.userMarker, tc.role, usr)
			}
		})
	}

	// Unknown role falls back to default templates
	data := TemplateData{AgentName: "alpha", AgentRole: "builder", Profile: &profile.Profile{}}
	out, err := Render("soul.md.tmpl", data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "You are **alpha**") {
		t.Error("fallback template missing agent name")
	}
}
