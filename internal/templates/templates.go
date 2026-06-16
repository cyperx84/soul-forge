package templates

import (
	"bytes"
	"embed"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/cyperx84/soul-forge/internal/config"
	"github.com/cyperx84/soul-forge/internal/profile"
)

//go:embed files
var filesFS embed.FS

// TemplateData is the context passed to every output template.
type TemplateData struct {
	AgentName string
	AgentRole string
	Channel   string
	Persona   *config.Persona  // merged role-default + author persona (voice/stance → SOUL.md)
	Operating []string         // operational rules (actions/ordering → AGENTS.md)
	Profile   *profile.Profile // facts about the human
	OutputDir string
}

// HasWorkingContext reports whether the profile carries any of the calibration the
// SOUL.md "Working With" section actually renders (the user's name alone — which only
// appears in the heading — does not count). Guards against an empty section heading.
func (d TemplateData) HasWorkingContext() bool {
	if d.Profile == nil {
		return false
	}
	i := d.Profile.Identity
	if i.CommunicationStyle != "" || i.TechnicalSkill != "" || i.Articulation != "" ||
		len(i.ExpertiseAreas) > 0 || len(i.LearningFocus) > 0 || i.WorkingHours != "" || i.Timezone != "" {
		return true
	}
	return d.Profile.WorkStyle.HasContent()
}

// UserName returns the human's name, or "this user" when unknown — so templates
// read naturally without sprinkling orDefault everywhere.
func (d TemplateData) UserName() string {
	if d.Profile != nil && d.Profile.Identity.Name != "" {
		return d.Profile.Identity.Name
	}
	return "this user"
}

// Render renders the named template (e.g. "soul.md.tmpl") with the given data.
func Render(name string, data TemplateData) (string, error) {
	content, err := filesFS.ReadFile("files/" + name)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", name, err)
	}

	tmpl, err := template.New(name).Funcs(funcMap()).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", name, err)
	}
	return collapseBlankRuns(buf.String()), nil
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		// orDefault returns def when val is empty.
		"orDefault": func(val, def string) string {
			if strings.TrimSpace(val) == "" {
				return def
			}
			return val
		},
		// bullets renders a markdown bullet list. Empty input yields "" so callers
		// can guard the whole section with {{if}} and avoid "Not specified" noise.
		"bullets": func(items []string) string {
			var b strings.Builder
			for _, item := range items {
				if strings.TrimSpace(item) != "" {
					fmt.Fprintf(&b, "- %s\n", item)
				}
			}
			return b.String()
		},
		// csv joins items with commas.
		"csv": func(items []string) string {
			return strings.Join(items, ", ")
		},
		// add1 returns i+1, for 1-based numbered lists over a 0-based range index.
		"add1": func(i int) int { return i + 1 },
		// aan returns the indefinite article ("a"/"an") that fits the next word.
		"aan": func(word string) string {
			w := strings.TrimSpace(word)
			if w == "" {
				return "a"
			}
			switch w[0] {
			case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
				return "an"
			}
			return "a"
		},
		// sortedMapKeys returns map keys in deterministic order.
		"sortedMapKeys": func(m map[string]string) []string {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return keys
		},
	}
}

// collapseBlankRuns squeezes runs of 2+ blank lines down to one, so graceful
// section omission never leaves big vertical gaps in the rendered markdown.
func collapseBlankRuns(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blanks := 0
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			blanks++
			if blanks > 1 {
				continue
			}
		} else {
			blanks = 0
		}
		out = append(out, ln)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}
