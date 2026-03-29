package cmd

import (
	"github.com/cyperx84/soul-forge/internal/interview"
	"github.com/spf13/cobra"
)

var (
	interviewProvider   string
	interviewModel      string
	interviewAPIKey     string
	interviewBaseURL    string
	interviewResume     bool
	interviewNoDotfiles bool
	interviewMaxTurns   int
	interviewOutput     string
)

var interviewCmd = &cobra.Command{
	Use:   "interview",
	Short: "Run an LLM-assisted conversational onboarding interview",
	Long: `Starts an interactive conversation with an LLM to build your profile.json.
The LLM adapts questions based on your answers, follows up on interesting threads,
and builds your profile incrementally. Much more natural than filling out a form.`,
	RunE: runInterview,
}

func init() {
	interviewCmd.Flags().StringVar(&interviewProvider, "provider", "openai", "LLM provider: openai, anthropic, ollama, openrouter")
	interviewCmd.Flags().StringVar(&interviewModel, "model", "gpt-4o-mini", "Model to use")
	interviewCmd.Flags().StringVar(&interviewAPIKey, "api-key", "", "API key (or set env vars)")
	interviewCmd.Flags().StringVar(&interviewBaseURL, "base-url", "", "Custom API base URL")
	interviewCmd.Flags().BoolVar(&interviewResume, "resume", false, "Resume a previous interview session")
	interviewCmd.Flags().BoolVar(&interviewNoDotfiles, "no-dotfiles", false, "Skip dotfiles context")
	interviewCmd.Flags().IntVar(&interviewMaxTurns, "max-turns", 30, "Maximum conversation turns")
	interviewCmd.Flags().StringVar(&interviewOutput, "output", ".soul-forge/profile.json", "Profile output path")
}

func runInterview(cmd *cobra.Command, args []string) error {
	cfg := interview.Config{
		Provider:   interviewProvider,
		Model:      interviewModel,
		APIKey:     interviewAPIKey,
		BaseURL:    interviewBaseURL,
		MaxTurns:   interviewMaxTurns,
		OutputPath: interviewOutput,
		NoDotfiles: interviewNoDotfiles,
		Resume:     interviewResume,
	}

	iv, err := interview.New(cfg)
	if err != nil {
		return err
	}

	return iv.Run(cmd.Context())
}
