package cmd

import (
	"fmt"

	"github.com/cyperx84/soul-forge/internal/config"
	"github.com/cyperx84/soul-forge/internal/rubric"
	"github.com/spf13/cobra"
)

var (
	rubricAgent string
	rubricAll   bool
)

var rubricCmd = &cobra.Command{
	Use:   "rubric",
	Short: "Emit a drift-test rubric to check a persona holds on a cheap model",
	Long: `Builds a deterministic drift-test rubric from an agent's persona — probes and
scoring criteria derived from its opinions, boundaries, and tensions.

soul-forge never calls a model, so it can't run the test; it emits the rubric for
your harness to run against a cheap model, scoring how well the agent stays in
character. This is the empirical complement to the static "audit" check.`,
	RunE:         runRubric,
	SilenceUsage: true,
}

func init() {
	rubricCmd.Flags().StringVar(&rubricAgent, "agent", "", "Build a rubric for a specific agent by name")
	rubricCmd.Flags().BoolVar(&rubricAll, "all", false, "Build a rubric for every agent")
}

func runRubric(cmd *cobra.Command, args []string) error {
	if rubricAgent != "" && rubricAll {
		return fmt.Errorf("--agent and --all are mutually exclusive")
	}
	if rubricAgent == "" && !rubricAll {
		return fmt.Errorf("specify --agent NAME or --all")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var agents []config.Agent
	if rubricAll {
		agents = cfg.Agents
	} else {
		for _, a := range cfg.Agents {
			if a.Name == rubricAgent {
				agents = append(agents, a)
				break
			}
		}
		if len(agents) == 0 {
			return fmt.Errorf("agent %q not found in %s", rubricAgent, configPath)
		}
	}

	for i, a := range agents {
		if i > 0 {
			fmt.Print("\n---\n\n")
		}
		fmt.Print(rubric.Build(a.Name, a.EffectivePersona()))
	}
	return nil
}
