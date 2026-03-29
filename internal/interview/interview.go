package interview

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cyperx84/soul-forge/internal/llm"
	"github.com/cyperx84/soul-forge/internal/profile"
)

// Config holds interview configuration.
type Config struct {
	Provider   string
	Model      string
	APIKey     string
	BaseURL    string
	MaxTurns   int
	OutputPath string
	NoDotfiles bool
	Resume     bool
}

// Interview runs the conversational onboarding flow.
type Interview struct {
	cfg    Config
	client llm.Client
}

// New creates a new Interview.
func New(cfg Config) (*Interview, error) {
	client, err := llm.NewClient(cfg.Provider, cfg.Model, cfg.APIKey, cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	return &Interview{cfg: cfg, client: client}, nil
}

// Run executes the interview loop.
func (iv *Interview) Run(ctx context.Context) error {
	var session *Session
	dotfilesPath := ".soul-forge/dotfiles.json"
	profilePath := iv.cfg.OutputPath

	if iv.cfg.Resume {
		var err error
		session, err = LoadSession()
		if err != nil {
			return fmt.Errorf("--resume: no existing session found: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Resuming session (%d turns, %d fields captured)\n",
			session.Turns, len(session.FieldsCaptured))
	} else {
		session = NewSession(iv.cfg.Provider, iv.cfg.Model)
	}

	systemPrompt := BuildSystemPrompt(dotfilesPath, profilePath, session, iv.cfg.NoDotfiles)
	extractor := NewExtractor(iv.client, iv.cfg.Model)

	// Build LLM message list
	llmMsgs := []llm.Message{
		{Role: "system", Content: systemPrompt},
	}
	for _, m := range session.Messages {
		llmMsgs = append(llmMsgs, llm.Message{Role: m.Role, Content: m.Content})
	}

	scanner := bufio.NewScanner(os.Stdin)

	// If fresh session, get opening message from LLM
	if len(session.Messages) == 0 {
		fmt.Fprintf(os.Stderr, "Starting interview with %s/%s...\n\n", iv.cfg.Provider, iv.cfg.Model)

		resp, err := iv.client.Chat(ctx, append(llmMsgs, llm.Message{
			Role:    "user",
			Content: "Hi! I'm ready to get started.",
		}), llm.ChatOpts{Temperature: 0.7, MaxTokens: 1024})
		if err != nil {
			return fmt.Errorf("initial message failed: %w", err)
		}

		fmt.Println(resp)
		fmt.Println()

		session.AppendMessage("assistant", resp)
		llmMsgs = append(llmMsgs, llm.Message{Role: "assistant", Content: resp})
		session.Save()
	}

	for session.Turns < iv.cfg.MaxTurns {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Check for manual exit commands
		lower := strings.ToLower(input)
		if lower == "done" || lower == "exit" || lower == "quit" || lower == "bye" {
			break
		}

		session.AppendMessage("user", input)
		llmMsgs = append(llmMsgs, llm.Message{Role: "user", Content: input})

		resp, err := iv.client.Chat(ctx, llmMsgs, llm.ChatOpts{
			Temperature: 0.7,
			MaxTokens:   1024,
		})
		if err != nil {
			return fmt.Errorf("LLM call failed: %w", err)
		}

		// Check for completion signal
		if strings.HasPrefix(resp, "[COMPLETE]") {
			resp = strings.TrimPrefix(resp, "[COMPLETE]")
			resp = strings.TrimSpace(resp)
			fmt.Println(resp)
			fmt.Println()
			session.AppendMessage("assistant", resp)
			break
		}

		fmt.Println(resp)
		fmt.Println()

		session.AppendMessage("assistant", resp)
		llmMsgs = append(llmMsgs, llm.Message{Role: "assistant", Content: resp})

		// Periodic extraction every 4 turns
		if session.Turns%4 == 0 {
			iv.extractAndSave(ctx, extractor, session, profilePath)
		}

		session.Save()
	}

	// Final extraction
	fmt.Fprintf(os.Stderr, "\nExtracting profile data...\n")
	iv.extractAndSave(ctx, extractor, session, profilePath)
	session.Save()

	// Show summary
	iv.showSummary(profilePath)

	return nil
}

func (iv *Interview) extractAndSave(ctx context.Context, ext *Extractor, session *Session, profilePath string) {
	msgs := session.ToLLMMessages()
	if len(msgs) < 2 {
		return
	}

	p, err := ext.Extract(ctx, msgs)
	if err != nil || p == nil {
		return
	}

	// Merge into existing profile or create new one
	existing := &profile.Profile{}
	if data, err := os.ReadFile(profilePath); err == nil {
		jsonErr := jsonUnmarshal(data, existing)
		if jsonErr != nil {
			existing = &profile.Profile{}
		}
	}

	// Use profile.Merge via temp files
	tmpIncoming := profilePath + ".tmpincoming"
	tmpExisting := profilePath + ".tmpexisting"

	// Write overlay
	if writeProfileJSON(tmpIncoming, p) != nil {
		return
	}
	// Write base
	if writeProfileJSON(tmpExisting, existing) != nil {
		os.Remove(tmpIncoming)
		return
	}

	// Merge: load both, merge in memory, write
	if err := profile.Merge(tmpIncoming, tmpExisting); err == nil {
		// Copy merged result to actual output
		data, err := os.ReadFile(tmpExisting)
		if err == nil {
			os.MkdirAll(".soul-forge", 0755)
			os.WriteFile(profilePath, data, 0644)
		}
	}

	os.Remove(tmpIncoming)
	os.Remove(tmpExisting)

	// Update fields captured
	merged := &profile.Profile{}
	if data, err := os.ReadFile(profilePath); err == nil {
		if jsonUnmarshal(data, merged) == nil {
			session.FieldsCaptured = merged.FieldsCaptured()
		}
	}
}

func (iv *Interview) showSummary(profilePath string) {
	p, err := profile.Load(profilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No profile data captured.\n")
		return
	}

	fields := p.FieldsCaptured()
	fmt.Fprintf(os.Stderr, "\n✓ Interview complete!\n")
	fmt.Fprintf(os.Stderr, "  Fields captured (%d): %s\n", len(fields), strings.Join(fields, ", "))
	fmt.Fprintf(os.Stderr, "  Profile saved to: %s\n", profilePath)
	fmt.Fprintf(os.Stderr, "\nNext: soul-forge generate --all\n")
}

func writeProfileJSON(path string, p *profile.Profile) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
