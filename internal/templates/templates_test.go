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
