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
