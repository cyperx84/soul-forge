package interview

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/cyperx84/soul-forge/internal/profile"
)

// msg is a local alias to avoid importing llm here.
type msg struct {
	Role    string
	Content string
}

// BuildSystemPrompt constructs the system prompt for the interview LLM.
func BuildSystemPrompt(dotfilesPath, profilePath string, session *Session, noDotfiles bool) string {
	var b strings.Builder

	b.WriteString("You are an onboarding interviewer for an AI agent system called soul-forge. ")
	b.WriteString("Your job is to learn about this person through natural, conversational dialogue.\n\n")

	b.WriteString("## Guidelines\n")
	b.WriteString("- Be conversational, warm, and genuinely curious — not robotic or form-like\n")
	b.WriteString("- Cover one topic at a time. Follow interesting threads.\n")
	b.WriteString("- Adapt to who they are: developer? go deep on tools. Creative? go deep on process.\n")
	b.WriteString("- Don't list questions. Weave topics naturally into conversation.\n")
	b.WriteString("- After each exchange, internally track which profile fields you've gathered.\n")
	b.WriteString("- When you have enough info across all areas, summarize what you learned and wrap up.\n")
	b.WriteString("- When wrapping up, start your message with [COMPLETE] so the system knows to stop.\n\n")

	b.WriteString("## Topics to Cover\n")
	b.WriteString("These are topic areas, NOT questions to read aloud:\n")
	b.WriteString("- Identity: name, role/title, background, timezone, working hours\n")
	b.WriteString("- Goals: what they want to achieve, learning focus areas\n")
	b.WriteString("- Communication: how they like to communicate, feedback preferences, collaboration style\n")
	b.WriteString("- Work style: preferences, workflow, decision-making approach, things to avoid\n")
	b.WriteString("- Technical: programming languages, tools, editors, terminal, OS, hardware\n")
	b.WriteString("- Output preferences: code style, documentation, commit messages, etc.\n\n")

	// Add dotfiles context if available
	if !noDotfiles && dotfilesPath != "" {
		if data, err := os.ReadFile(dotfilesPath); err == nil {
			var df map[string]interface{}
			if json.Unmarshal(data, &df) == nil {
				b.WriteString("## Dotfiles Context (already known from scanning their dotfiles)\n")
				b.WriteString("```json\n")
				b.Write(data)
				b.WriteString("\n```\n")
				b.WriteString("Use this info naturally — e.g. \"I see you use Neovim...\" instead of asking.\n\n")
			}
		}
	}

	// Add partial profile context if resuming
	if profilePath != "" {
		if data, err := os.ReadFile(profilePath); err == nil {
			var p profile.Profile
			if json.Unmarshal(data, &p) == nil {
				fields := p.FieldsCaptured()
				if len(fields) > 0 {
					b.WriteString("## Already Known (from previous interview)\n")
					b.WriteString("Fields already captured: " + strings.Join(fields, ", ") + "\n")
					b.WriteString("```json\n")
					b.Write(data)
					b.WriteString("\n```\n")
					b.WriteString("Continue from where we left off. Don't re-ask about topics already covered.\n\n")
				}
			}
		}
	}

	// Resume context
	if session != nil && len(session.FieldsCaptured) > 0 {
		b.WriteString("## Resume Note\n")
		b.WriteString("This is a resumed session. We've already had " + fmt.Sprintf("%d", session.Turns) + " turns.\n")
		b.WriteString("Fields captured so far: " + strings.Join(session.FieldsCaptured, ", ") + "\n\n")
	}

	b.WriteString("## Profile Schema\n")
	b.WriteString("Build toward populating this JSON structure:\n")
	b.WriteString("```json\n")
	b.WriteString(`{
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
}` + "\n```\n")

	return b.String()
}
