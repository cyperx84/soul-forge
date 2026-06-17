package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cyperx84/soul-forge/internal/config"
	"github.com/cyperx84/soul-forge/internal/profile"
	"github.com/cyperx84/soul-forge/internal/soulmd"
	"github.com/spf13/cobra"
)

const configPath = "soul-forge.yaml"

var (
	importMerge      bool
	importFromSoulMD string
	importAgent      string
)

var importCmd = &cobra.Command{
	Use:   "import [profile.json]",
	Short: "Import a structured profile JSON (or a soul.md persona)",
	Long: `Imports a structured profile JSON into .soul-forge/profile.json.

A profile.json may carry a top-level "agents" array; those personas are routed
into soul-forge.yaml (matched by name). Alternatively, --from-soul-md <dir> ingests
a soul.md-format persona directory (aaronjmars/soul.md: SOUL.md + STYLE.md +
examples/) and applies it to an agent (--agent, or the sole agent in the fleet).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runImport,
}

func init() {
	importCmd.Flags().BoolVar(&importMerge, "merge", false, "Merge with existing profile instead of overwriting")
	importCmd.Flags().StringVar(&importFromSoulMD, "from-soul-md", "", "Import a soul.md persona directory and apply it to an agent")
	importCmd.Flags().StringVar(&importAgent, "agent", "", "Target agent for --from-soul-md (defaults to the sole agent)")
}

func runImport(cmd *cobra.Command, args []string) error {
	if importFromSoulMD != "" {
		return runImportSoulMD()
	}
	if len(args) != 1 {
		return fmt.Errorf("import requires a <profile.json> argument (or use --from-soul-md)")
	}
	src := args[0]
	dst := ".soul-forge/profile.json"

	if importMerge {
		if err := profile.Merge(src, dst); err != nil {
			return fmt.Errorf("merge failed: %w", err)
		}
		fmt.Printf("✓ Merged %s into %s\n", src, dst)
	} else {
		if err := profile.Import(src, dst); err != nil {
			return fmt.Errorf("import failed: %w", err)
		}
		fmt.Printf("✓ Imported %s → %s\n", src, dst)
	}

	// The payload may also carry per-agent personas under a top-level `agents`
	// array. profile.json ignores them; route them into soul-forge.yaml so the
	// voice/opinions/examples from onboarding actually reach each SOUL.md.
	seeds, err := readAgentSeeds(src)
	if err != nil {
		return err
	}
	if len(seeds) > 0 {
		updated, added, err := config.ApplyPersonas(configPath, seeds)
		if err != nil {
			fmt.Printf("⚠ %d persona(s) not applied to %s: %v\n", len(seeds), configPath, err)
		} else {
			fmt.Printf("✓ Personas → %s (%d updated, %d added)\n", configPath, updated, added)
		}
	}
	return nil
}

// runImportSoulMD ingests a soul.md persona directory and applies it to one agent.
func runImportSoulMD() error {
	persona, err := soulmd.Parse(importFromSoulMD)
	if err != nil {
		return fmt.Errorf("parse soul.md: %w", err)
	}
	target, err := resolveAgent(importAgent)
	if err != nil {
		return err
	}
	updated, added, err := config.ApplyPersonas(configPath, []config.AgentSeed{{Name: target, Persona: persona}})
	if err != nil {
		return fmt.Errorf("apply persona: %w", err)
	}
	_ = added
	fmt.Printf("✓ soul.md → %s persona for agent %q (in %s)\n",
		map[bool]string{true: "updated", false: "set"}[updated > 0], target, configPath)
	return nil
}

// resolveAgent returns the named agent, or the sole agent when name is empty.
func resolveAgent(name string) (string, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return "", err
	}
	if name != "" {
		for _, a := range cfg.Agents {
			if a.Name == name {
				return name, nil
			}
		}
		return "", fmt.Errorf("agent %q not found in %s", name, configPath)
	}
	if len(cfg.Agents) == 1 {
		return cfg.Agents[0].Name, nil
	}
	return "", fmt.Errorf("multiple agents in %s — specify which with --agent", configPath)
}

// readAgentSeeds pulls the optional top-level `agents` array out of the import
// payload. Unknown to profile.json, it's where designed personas ride in.
func readAgentSeeds(src string) ([]config.AgentSeed, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", src, err)
	}
	var payload struct {
		Agents []config.AgentSeed `json:"agents"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse %s: %w", src, err)
	}
	return payload.Agents, nil
}
