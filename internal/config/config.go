package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Version is set by goreleaser ldflags at build time.
var Version = "dev"

type Config struct {
	OutputDir string  `yaml:"output_dir"`
	Dotfiles  string  `yaml:"dotfiles,omitempty"`
	Agents    []Agent `yaml:"agents"`
}

type Agent struct {
	Name    string `yaml:"name"`
	Role    string `yaml:"role"`
	Channel string `yaml:"channel,omitempty"`

	// Persona shapes the agent's SOUL.md. It is optional: role templates supply
	// sensible defaults, and any fields set here override or augment them.
	Persona *Persona `yaml:"persona,omitempty"`
}

// Persona gives an agent its own identity and voice — the heart of a good SOUL.md.
// Every field is optional; omitted fields fall back to role-template defaults.
type Persona struct {
	Vibe       string     `yaml:"vibe,omitempty"`       // one-line identity card for IDENTITY.md
	Emoji      string     `yaml:"emoji,omitempty"`      // signature emoji for IDENTITY.md
	Backstory  string     `yaml:"backstory,omitempty"`  // one line: "a senior SRE with 12 years in cloud infra"
	Voice      string     `yaml:"voice,omitempty"`      // tone: "dry, precise, allergic to filler"
	Opinions   []string   `yaml:"opinions,omitempty"`   // takes the agent holds ("strong opinions, loosely held")
	Principles []string   `yaml:"principles,omitempty"` // stance/communication posture (NOT operational rules — those live in AGENTS.md)
	Boundaries []string   `yaml:"boundaries,omitempty"` // identity/integrity lines the agent won't cross
	Avoid      []string   `yaml:"avoid,omitempty"`      // stylistic anti-patterns ("no corporate hedging")
	Examples   []Exchange `yaml:"examples,omitempty"`   // few-shot exchanges that lock in voice
}

// Exchange is a single few-shot example: a prompt and the response the agent should model.
type Exchange struct {
	Prompt   string `yaml:"prompt"`
	Response string `yaml:"response"`
}

// HasContent reports whether the persona carries any author-supplied content.
func (p *Persona) HasContent() bool {
	if p == nil {
		return false
	}
	return p.Vibe != "" || p.Emoji != "" || p.Backstory != "" || p.Voice != "" || len(p.Opinions) > 0 ||
		len(p.Principles) > 0 || len(p.Boundaries) > 0 || len(p.Avoid) > 0 || len(p.Examples) > 0
}

func Default() *Config {
	return &Config{
		OutputDir: "agents",
		Agents: []Agent{
			{
				Name:    "assistant",
				Role:    "general",
				Channel: "main",
			},
		},
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return &cfg, nil
}

func Write(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	header := "# soul-forge configuration\n# See: soul-forge --help\n\n"
	return os.WriteFile(path, append([]byte(header), data...), 0644)
}

// Validate checks the config for common mistakes.
func (c *Config) Validate() error {
	if c.OutputDir == "" {
		return fmt.Errorf("output_dir must not be empty")
	}
	if len(c.Agents) == 0 {
		return fmt.Errorf("at least one agent must be defined")
	}
	seen := make(map[string]struct{}, len(c.Agents))
	for i, a := range c.Agents {
		if a.Name == "" {
			return fmt.Errorf("agent[%d]: name must not be empty", i)
		}
		if _, dup := seen[a.Name]; dup {
			return fmt.Errorf("duplicate agent name %q", a.Name)
		}
		seen[a.Name] = struct{}{}
	}
	return nil
}
