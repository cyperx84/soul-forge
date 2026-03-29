package templates

import (
	"bytes"
	"embed"
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/cyperx84/soul-forge/internal/profile"
)

//go:embed files
var filesFS embed.FS

type TemplateData struct {
	AgentName string
	AgentRole string
	Channel   string
	Profile   *profile.Profile
	OutputDir string
}

// Render renders a template by name with the given data.
// For soul.md.tmpl and user.md.tmpl, it checks for a role-specific variant
// (e.g. soul.coding.md.tmpl) and uses it if present, falling back to the default.
func Render(name string, data TemplateData) (string, error) {
	resolved := resolveTemplateName(name, data.AgentRole)
	return renderTemplate(resolved, data)
}

// resolveTemplateName returns a role-specific template name if one exists for the
// given base name and role, otherwise returns the original name unchanged.
func resolveTemplateName(name, role string) string {
	if role == "" {
		return name
	}
	// Only attempt role lookup for the two primary templates.
	if name != "soul.md.tmpl" && name != "user.md.tmpl" {
		return name
	}
	base := strings.TrimSuffix(name, ".md.tmpl") // "soul" or "user"
	candidate := fmt.Sprintf("%s.%s.md.tmpl", base, role)
	if _, err := filesFS.Open("files/" + candidate); err == nil {
		return candidate
	}
	return name
}

func renderTemplate(name string, data TemplateData) (string, error) {
	path := fmt.Sprintf("files/%s", name)
	content, err := filesFS.ReadFile(path)
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
	return buf.String(), nil
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"orDefault": func(val, def string) string {
			if val == "" {
				return def
			}
			return val
		},
		"joinLines": func(items []string) string {
			if len(items) == 0 {
				return "- Not specified\n"
			}
			var b strings.Builder
			for _, item := range items {
				fmt.Fprintf(&b, "- %s\n", item)
			}
			return b.String()
		},
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
