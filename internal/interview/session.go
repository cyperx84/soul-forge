package interview

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const sessionDir = ".soul-forge"

// Session holds interview state for persistence and resume.
type Session struct {
	StartedAt      string          `json:"started_at"`
	UpdatedAt      string          `json:"updated_at"`
	Provider       string          `json:"provider"`
	Model          string          `json:"model"`
	Turns          int             `json:"turns"`
	Messages       []SessionMsg   `json:"messages"`
	FieldsCaptured []string        `json:"fields_captured"`
}

// SessionMsg is a single message in the session history.
type SessionMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LoadSession reads a session from .soul-forge/session.json.
func LoadSession() (*Session, error) {
	path := filepath.Join(sessionDir, "session.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse session: %w", err)
	}
	return &s, nil
}

// Save persists the session to .soul-forge/session.json.
func (s *Session) Save() error {
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		return err
	}
	s.UpdatedAt = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(sessionDir, "session.json"), data, 0644)
}

// NewSession creates a fresh session.
func NewSession(provider, model string) *Session {
	return &Session{
		StartedAt: time.Now().Format(time.RFC3339),
		UpdatedAt: time.Now().Format(time.RFC3339),
		Provider:  provider,
		Model:     model,
	}
}

// AppendMessage adds a message and increments the turn counter for user messages.
func (s *Session) AppendMessage(role, content string) {
	s.Messages = append(s.Messages, SessionMsg{Role: role, Content: content})
	if role == "user" {
		s.Turns++
	}
}

// ToLLMMessages converts session messages to LLM Message format.
func (s *Session) ToLLMMessages() []msg {
	msgs := make([]msg, len(s.Messages))
	for i, m := range s.Messages {
		msgs[i] = msg{Role: m.Role, Content: m.Content}
	}
	return msgs
}
