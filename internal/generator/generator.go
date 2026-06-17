package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/cyperx84/soul-forge/internal/config"
	"github.com/cyperx84/soul-forge/internal/manifest"
	"github.com/cyperx84/soul-forge/internal/profile"
	tmplpkg "github.com/cyperx84/soul-forge/internal/templates"
)

var validAgentName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

// output describes one generated file: which template renders it, and whether an
// existing copy should be preserved on regenerate (MEMORY.md accumulates over time).
type output struct {
	filename string
	template string
	preserve bool
}

// outputs is the full set of files generated per agent, in display order.
var outputs = []output{
	{"SOUL.md", "soul.md.tmpl", false},
	{"IDENTITY.md", "identity.md.tmpl", false},
	{"USER.md", "user.md.tmpl", false},
	{"AGENTS.md", "agents.md.tmpl", false},
	{"TOOLS.md", "tools.md.tmpl", false},
	{"MEMORY.md", "memory.md.tmpl", true}, // seed once; never clobber learned memory
}

// Generate renders and writes all agent files for a single agent. With dryRun set,
// it prints the rendered output to stdout instead of writing anything.
func Generate(cfg *config.Config, prof *profile.Profile, agent config.Agent, dryRun bool) error {
	if !validAgentName.MatchString(agent.Name) {
		return fmt.Errorf("invalid agent name %q: must contain only alphanumeric characters and hyphens, and start with alphanumeric", agent.Name)
	}

	data := tmplpkg.TemplateData{
		AgentName: agent.Name,
		AgentRole: agent.Role,
		Channel:   agent.Channel,
		Persona:   agent.EffectivePersona(),
		Operating: config.DefaultOperatingRules(agent.Role),
		Profile:   prof,
		OutputDir: cfg.OutputDir,
	}

	rendered := make(map[string]string, len(outputs))
	for _, o := range outputs {
		content, err := tmplpkg.Render(o.template, data)
		if err != nil {
			return fmt.Errorf("render %s: %w", o.filename, err)
		}
		rendered[o.filename] = content
	}

	// soul.json is structured data, built directly rather than from a text template.
	soulJSON, err := manifest.Build(cfg, agent)
	if err != nil {
		return fmt.Errorf("build soul.json: %w", err)
	}
	rendered["soul.json"] = string(soulJSON)

	if dryRun {
		for _, o := range outputs {
			fmt.Printf("=== %s (%s) ===\n%s\n", o.filename, agent.Name, rendered[o.filename])
		}
		fmt.Printf("=== soul.json (%s) ===\n%s\n", agent.Name, rendered["soul.json"])
		return nil
	}

	agentDir := filepath.Join(cfg.OutputDir, agent.Name)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}
	// AGENTS.md points the agent at a memory/ daily-log dir; make sure it exists.
	if err := os.MkdirAll(filepath.Join(agentDir, "memory"), 0755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}

	for _, o := range outputs {
		path := filepath.Join(agentDir, o.filename)
		if o.preserve {
			if _, err := os.Stat(path); err == nil {
				fmt.Printf("• Kept %s (preserved existing)\n", path)
				continue
			}
		}
		if err := os.WriteFile(path, []byte(rendered[o.filename]), 0644); err != nil {
			return fmt.Errorf("write %s: %w", o.filename, err)
		}
		fmt.Printf("✓ Wrote %s\n", path)
	}

	soulPath := filepath.Join(agentDir, "soul.json")
	if err := os.WriteFile(soulPath, []byte(rendered["soul.json"]), 0644); err != nil {
		return fmt.Errorf("write soul.json: %w", err)
	}
	fmt.Printf("✓ Wrote %s\n", soulPath)

	return nil
}
