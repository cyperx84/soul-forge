package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cyperx84/soul-forge/internal/questionnaire"
	"github.com/spf13/cobra"
)

var (
	questionsPart   int
	questionsFormat string
)

var questionsCmd = &cobra.Command{
	Use:   "questions",
	Short: "Output the onboarding questionnaire",
	Long:  `Outputs the questionnaire as markdown (default) or JSON.`,
	RunE:  runQuestions,
}

func init() {
	questionsCmd.Flags().IntVar(&questionsPart, "part", 0, "Output only part N (1, 2, or 3). Default: all parts.")
	questionsCmd.Flags().StringVar(&questionsFormat, "format", "md", "Output format: md or json")
}

func runQuestions(cmd *cobra.Command, args []string) error {
	parts, err := questionnaire.Load(questionsPart)
	if err != nil {
		return err
	}

	switch questionsFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(parts)
	case "md":
		for _, p := range parts {
			fmt.Print(p.Markdown())
		}
		return nil
	default:
		return fmt.Errorf("unknown format %q — use md or json", questionsFormat)
	}
}
