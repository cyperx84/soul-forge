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

	// Author and License populate the generated soul.json manifest (SoulSpec).
	// Both optional; a SoulSpec package wants a license, so audit nudges when unset.
	Author  string `yaml:"author,omitempty"`
	License string `yaml:"license,omitempty"`
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
// JSON tags mirror the YAML so the onboarding payload (see AgentSeed) can carry
// personas straight from `soul-forge import` into this struct.
type Persona struct {
	Vibe       string     `yaml:"vibe,omitempty" json:"vibe,omitempty"`                         // one-line identity card for IDENTITY.md
	Emoji      string     `yaml:"emoji,omitempty" json:"emoji,omitempty"`                       // signature emoji for IDENTITY.md
	Backstory  string     `yaml:"backstory,omitempty" json:"backstory,omitempty"`               // one line: "a senior SRE with 12 years in cloud infra"
	Voice      string     `yaml:"voice,omitempty" json:"voice,omitempty"`                       // tone: "dry, precise, allergic to filler"
	Opinions   []string   `yaml:"opinions,omitempty" json:"opinions,omitempty"`                 // takes the agent holds ("strong opinions, loosely held")
	Tensions   []string   `yaml:"tensions,omitempty" json:"tensions,omitempty"`                 // honest contradictions ("I value speed but distrust anything I can't undo")
	Principles []string   `yaml:"principles,omitempty" json:"principles,omitempty"`             // stance/communication posture (NOT operational rules — those live in AGENTS.md)
	Boundaries []string   `yaml:"boundaries,omitempty" json:"boundaries,omitempty"`             // identity/integrity lines the agent won't cross
	Avoid      []string   `yaml:"avoid,omitempty" json:"avoid,omitempty"`                       // stylistic anti-patterns ("no corporate hedging")
	Examples   []Exchange `yaml:"examples,omitempty" json:"examples,omitempty"`                 // few-shot exchanges that lock in voice
	Counters   []Exchange `yaml:"counter_examples,omitempty" json:"counter_examples,omitempty"` // how NOT to respond — negative calibration
}

// Exchange is a single few-shot example: a prompt, the response the agent should
// model (or, for a counter-example, must avoid), and an optional note naming the
// move it demonstrates ("leads with the caveat, not an apology"). The note is the
// calibration signal — it tells the agent *why* the example reads the way it does.
type Exchange struct {
	Prompt   string `yaml:"prompt" json:"prompt"`
	Response string `yaml:"response" json:"response"`
	Note     string `yaml:"note,omitempty" json:"note,omitempty"`
}

// AgentSeed is a per-agent persona design produced by the onboarding interview.
// It travels in the import payload's top-level `agents` array; `soul-forge import`
// routes these into soul-forge.yaml (matched by name) so the rich voice/opinions/
// boundaries/examples drawn out of the user actually reach an agent's SOUL.md,
// instead of being stranded as facts in profile.json.
type AgentSeed struct {
	Name    string   `json:"name"`
	Role    string   `json:"role,omitempty"`
	Persona *Persona `json:"persona,omitempty"`
}

// HasContent reports whether the persona carries any author-supplied content.
func (p *Persona) HasContent() bool {
	if p == nil {
		return false
	}
	return p.Vibe != "" || p.Emoji != "" || p.Backstory != "" || p.Voice != "" || len(p.Opinions) > 0 ||
		len(p.Tensions) > 0 || len(p.Principles) > 0 || len(p.Boundaries) > 0 || len(p.Avoid) > 0 ||
		len(p.Examples) > 0 || len(p.Counters) > 0
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

// ApplyPersonas merges per-agent persona seeds into the config at path. An agent
// that already exists has its persona set and its role updated when the seed names
// one; an unknown agent name is appended (defaulting to the "general" role). It
// returns how many agents were updated vs. added. Seeds with an empty persona are
// skipped for existing agents so a partial payload never wipes a hand-tuned persona.
func ApplyPersonas(path string, seeds []AgentSeed) (updated, added int, err error) {
	if len(seeds) == 0 {
		return 0, 0, nil
	}
	cfg, err := Load(path)
	if err != nil {
		return 0, 0, err
	}
	index := make(map[string]int, len(cfg.Agents))
	for i, a := range cfg.Agents {
		index[a.Name] = i
	}
	for _, s := range seeds {
		if s.Name == "" {
			return updated, added, fmt.Errorf("persona seed is missing an agent name")
		}
		if i, ok := index[s.Name]; ok {
			if s.Persona != nil {
				cfg.Agents[i].Persona = s.Persona
			}
			if s.Role != "" {
				cfg.Agents[i].Role = s.Role
			}
			updated++
			continue
		}
		role := s.Role
		if role == "" {
			role = "general"
		}
		cfg.Agents = append(cfg.Agents, Agent{Name: s.Name, Role: role, Persona: s.Persona})
		index[s.Name] = len(cfg.Agents) - 1
		added++
	}
	if err := cfg.Validate(); err != nil {
		return updated, added, err
	}
	return updated, added, Write(path, cfg)
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
