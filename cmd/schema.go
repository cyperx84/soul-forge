package cmd

import (
	"fmt"

	"github.com/cyperx84/soul-forge/internal/schema"
	"github.com/spf13/cobra"
)

var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Print the JSON Schema for profile.json",
	Long: `Prints the machine-readable JSON Schema describing .soul-forge/profile.json.

This is what makes soul-forge harness-agnostic: an agent harness conducts the
onboarding interview with its OWN LLM, builds a profile that conforms to this
schema, then pipes it through 'soul-forge import' and 'soul-forge generate'.
soul-forge never calls a model provider itself.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), schema.ProfileJSONSchema())
		return err
	},
}
