package cmd

import (
	"fmt"

	"github.com/cyperx84/soul-forge/internal/animation"
	"github.com/cyperx84/soul-forge/internal/config"
	"github.com/cyperx84/soul-forge/internal/generator"
	"github.com/cyperx84/soul-forge/internal/profile"
	"github.com/spf13/cobra"
)

var (
	generateAgent  string
	generateAll    bool
	generateDryRun bool
	generateNoAnim bool
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate USER.md and SOUL.md for agents",
	Long:  `Reads profile from .soul-forge/profile.json and agent defs from soul-forge.yaml, then generates USER.md and SOUL.md per agent.`,
	RunE:  runGenerate,
}

func init() {
	generateCmd.Flags().StringVar(&generateAgent, "agent", "", "Generate for a specific agent by name")
	generateCmd.Flags().BoolVar(&generateAll, "all", false, "Generate for all agents")
	generateCmd.Flags().BoolVar(&generateDryRun, "dry-run", false, "Print to stdout without writing files")
	generateCmd.Flags().BoolVar(&generateNoAnim, "no-animation", false, "Skip the forge animation")
}

func runGenerate(cmd *cobra.Command, args []string) error {
	if generateAgent != "" && generateAll {
		return fmt.Errorf("--agent and --all are mutually exclusive")
	}
	if generateAgent == "" && !generateAll {
		return fmt.Errorf("specify --agent NAME or --all")
	}

	cfg, err := config.Load("soul-forge.yaml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	prof, err := profile.Load(".soul-forge/profile.json")
	if err != nil {
		return fmt.Errorf("failed to load profile: %w", err)
	}

	if !generateDryRun && !generateNoAnim && animation.IsTTY() {
		animation.PlayForge()
	}

	var agents []config.Agent
	if generateAll {
		agents = cfg.Agents
	} else {
		for _, a := range cfg.Agents {
			if a.Name == generateAgent {
				agents = append(agents, a)
				break
			}
		}
		if len(agents) == 0 {
			return fmt.Errorf("agent %q not found in soul-forge.yaml", generateAgent)
		}
	}

	for _, agent := range agents {
		if err := generator.Generate(cfg, prof, agent, generateDryRun); err != nil {
			return fmt.Errorf("generate %s: %w", agent.Name, err)
		}
	}
	return nil
}
