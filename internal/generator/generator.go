package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/cyperx84/soul-forge/internal/config"
	"github.com/cyperx84/soul-forge/internal/profile"
	tmplpkg "github.com/cyperx84/soul-forge/internal/templates"
)

var validAgentName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`)

func Generate(cfg *config.Config, prof *profile.Profile, agent config.Agent, dryRun bool) error {
	if !validAgentName.MatchString(agent.Name) {
		return fmt.Errorf("invalid agent name %q: must contain only alphanumeric characters and hyphens, and start with alphanumeric", agent.Name)
	}

	data := tmplpkg.TemplateData{
		AgentName: agent.Name,
		AgentRole: agent.Role,
		Channel:   agent.Channel,
		Profile:   prof,
		OutputDir: cfg.OutputDir,
	}

	userMD, err := tmplpkg.Render("user.md.tmpl", data)
	if err != nil {
		return fmt.Errorf("render USER.md: %w", err)
	}

	soulMD, err := tmplpkg.Render("soul.md.tmpl", data)
	if err != nil {
		return fmt.Errorf("render SOUL.md: %w", err)
	}

	if dryRun {
		fmt.Printf("=== USER.md (%s) ===\n%s\n", agent.Name, userMD)
		fmt.Printf("=== SOUL.md (%s) ===\n%s\n", agent.Name, soulMD)
		return nil
	}

	agentDir := filepath.Join(cfg.OutputDir, agent.Name)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("create agent dir: %w", err)
	}

	userPath := filepath.Join(agentDir, "USER.md")
	if err := os.WriteFile(userPath, []byte(userMD), 0644); err != nil {
		return fmt.Errorf("write USER.md: %w", err)
	}
	fmt.Printf("✓ Wrote %s\n", userPath)

	soulPath := filepath.Join(agentDir, "SOUL.md")
	if err := os.WriteFile(soulPath, []byte(soulMD), 0644); err != nil {
		return fmt.Errorf("write SOUL.md: %w", err)
	}
	fmt.Printf("✓ Wrote %s\n", soulPath)

	return nil
}
