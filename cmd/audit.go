package cmd

import (
	"errors"
	"fmt"

	"github.com/cyperx84/soul-forge/internal/audit"
	"github.com/cyperx84/soul-forge/internal/config"
	"github.com/spf13/cobra"
)

// errAuditFailed is returned when audit finds issues. root.Execute() handles
// the exit code without printing a redundant error message.
var errAuditFailed = errors.New("audit found issues")

var (
	auditAgent string
	auditAll   bool
)

var auditCmd = &cobra.Command{
	Use:          "audit",
	Short:        "Audit USER.md and SOUL.md files for issues",
	Long:         `Checks for missing files, empty sections, broken references, and staleness.`,
	RunE:         runAudit,
	SilenceUsage: true,
}

func init() {
	auditCmd.Flags().StringVar(&auditAgent, "agent", "", "Audit a specific agent by name")
	auditCmd.Flags().BoolVar(&auditAll, "all", false, "Audit all agents")
}

func runAudit(cmd *cobra.Command, args []string) error {
	if auditAgent != "" && auditAll {
		return fmt.Errorf("--agent and --all are mutually exclusive")
	}
	if auditAgent == "" && !auditAll {
		return fmt.Errorf("specify --agent NAME or --all")
	}

	cfg, err := config.Load("soul-forge.yaml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var agents []config.Agent
	if auditAll {
		agents = cfg.Agents
	} else {
		for _, a := range cfg.Agents {
			if a.Name == auditAgent {
				agents = append(agents, a)
				break
			}
		}
		if len(agents) == 0 {
			return fmt.Errorf("agent %q not found in soul-forge.yaml", auditAgent)
		}
	}

	results := audit.Run(cfg, agents)
	hasIssues := false
	for _, r := range results {
		fmt.Print(r.Format())
		if r.HasIssues() {
			hasIssues = true
		}
	}

	if hasIssues {
		return errAuditFailed
	}
	return nil
}
