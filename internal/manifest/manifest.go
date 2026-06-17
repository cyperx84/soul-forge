// Package manifest builds the soul.json manifest — the machine-readable "passport"
// for a generated agent. Every serious agent-personality convention ships a manifest
// (SoulSpec's soul.json, TavernAI character cards, Letta memory blocks); soul-forge
// emitting one makes its output an installable SoulSpec-compatible package and lets it
// declare framework compatibility, version, and license alongside the markdown files.
//
// The shape follows SoulSpec v0.5 (github.com/clawsouls/soulspec). Fields soul-forge
// can't know are defaulted conservatively or omitted rather than guessed.
package manifest

import (
	"encoding/json"
	"strings"

	"github.com/cyperx84/soul-forge/internal/config"
)

const (
	specVersion    = "0.5"
	defaultVersion = "0.1.0"
)

// Manifest is the subset of SoulSpec v0.5 soul.json that soul-forge populates.
type Manifest struct {
	SpecVersion   string            `json:"specVersion"`
	Name          string            `json:"name"`
	DisplayName   string            `json:"displayName"`
	Version       string            `json:"version"`
	Description   string            `json:"description,omitempty"`
	Author        *Author           `json:"author,omitempty"`
	License       string            `json:"license,omitempty"`
	Tags          []string          `json:"tags,omitempty"`
	Category      string            `json:"category"`
	Compatibility Compatibility     `json:"compatibility"`
	Files         map[string]string `json:"files"`
}

type Author struct {
	Name string `json:"name"`
}

type Compatibility struct {
	Frameworks []string `json:"frameworks"`
}

// Build returns the soul.json bytes for one agent, given the fleet config.
func Build(cfg *config.Config, agent config.Agent) ([]byte, error) {
	p := agent.EffectivePersona()
	m := Manifest{
		SpecVersion: specVersion,
		Name:        agent.Name,
		DisplayName: agent.Name,
		Version:     defaultVersion,
		Description: describe(p),
		License:     cfg.License,
		Tags:        []string{config.CanonicalRole(agent.Role)},
		Category:    config.CanonicalRole(agent.Role),
		Compatibility: Compatibility{
			Frameworks: []string{"openclaw", "hermes"},
		},
		Files: map[string]string{
			"soul":     "SOUL.md",
			"identity": "IDENTITY.md",
			"user":     "USER.md",
			"agents":   "AGENTS.md",
			"tools":    "TOOLS.md",
			"memory":   "MEMORY.md",
		},
	}
	if cfg.Author != "" {
		m.Author = &Author{Name: cfg.Author}
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// describe builds a <=160-char description from the persona's vibe or backstory.
func describe(p *config.Persona) string {
	d := p.Vibe
	if d == "" {
		d = p.Backstory
	}
	d = strings.TrimSpace(d)
	if len(d) > 160 {
		d = strings.TrimSpace(string([]rune(d)[:159])) + "…"
	}
	return d
}
