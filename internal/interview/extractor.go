package interview

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cyperx84/soul-forge/internal/llm"
	"github.com/cyperx84/soul-forge/internal/profile"
)

// Extractor pulls structured profile data from conversation history.
type Extractor struct {
	client llm.Client
	model  string
}

// NewExtractor creates a new profile extractor.
func NewExtractor(client llm.Client, model string) *Extractor {
	return &Extractor{client: client, model: model}
}

// Extract sends conversation history to the LLM and asks it to extract profile fields.
func (e *Extractor) Extract(ctx context.Context, messages []msg) (*profile.Profile, error) {
	prompt := buildExtractionPrompt()

	// Convert to LLM messages
	llmMsgs := []llm.Message{
		{Role: "system", Content: prompt},
	}

	// Include conversation as a single user message to keep it simple
	var convo strings.Builder
	for _, m := range messages {
		convo.WriteString(fmt.Sprintf("%s: %s\n\n", m.Role, m.Content))
	}

	llmMsgs = append(llmMsgs, llm.Message{
		Role:    "user",
		Content: "Here is the conversation to extract from:\n\n" + convo.String(),
	})

	resp, err := e.client.Chat(ctx, llmMsgs, llm.ChatOpts{
		Temperature: 0.1,
		MaxTokens:   4096,
	})
	if err != nil {
		return nil, fmt.Errorf("extraction call failed: %w", err)
	}

	// Parse JSON from response — handle markdown code blocks
	jsonStr := resp
	if idx := strings.Index(jsonStr, "```json"); idx != -1 {
		jsonStr = jsonStr[idx+7:]
		if end := strings.Index(jsonStr, "```"); end != -1 {
			jsonStr = jsonStr[:end]
		}
	} else if idx := strings.Index(jsonStr, "```"); idx != -1 {
		jsonStr = jsonStr[idx+3:]
		if end := strings.Index(jsonStr, "```"); end != -1 {
			jsonStr = jsonStr[:end]
		}
	}
	jsonStr = strings.TrimSpace(jsonStr)

	var p profile.Profile
	if err := json.Unmarshal([]byte(jsonStr), &p); err != nil {
		// Return empty profile rather than failing the interview
		return nil, nil
	}

	return &p, nil
}

func buildExtractionPrompt() string {
	return `You are a data extraction assistant. Extract structured profile data from the conversation below.

Return ONLY valid JSON matching this schema. Only include fields you are confident about based on the conversation. Leave uncertain fields empty/null/omitted.

Schema:
{
  "identity": {
    "name": "", "role": "", "background": "",
    "goals": [], "communication_style": "",
    "expertise_areas": [], "learning_focus": [],
    "working_hours": "", "timezone": ""
  },
  "work_style": {
    "preferences": [], "workflow": "", "decision_style": "",
    "feedback_style": "", "collab_style": "",
    "tools": [], "languages": [], "do_not_do": [],
    "output_preferences": {}
  },
  "environment": {
    "os": "", "shell": "", "editor": "", "terminal": "",
    "hardware": "", "package_manager": "", "dotfiles_repo": "",
    "key_tools": [], "aliases": []
  }
}

Return the JSON object directly, no explanation.`
}
