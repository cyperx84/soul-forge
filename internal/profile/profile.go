package profile

import (
	"encoding/json"
	"fmt"
	"os"
)

type Profile struct {
	Identity    Identity    `json:"identity"`
	WorkStyle   WorkStyle   `json:"work_style"`
	Environment Environment `json:"environment"`
	UpdatedAt   string      `json:"updated_at,omitempty"`
}

type Identity struct {
	Name               string   `json:"name,omitempty"`
	Role               string   `json:"role,omitempty"`
	Background         string   `json:"background,omitempty"`
	Goals              []string `json:"goals,omitempty"`
	CommunicationStyle string   `json:"communication_style,omitempty"`
	ExpertiseAreas     []string `json:"expertise_areas,omitempty"`
	LearningFocus      []string `json:"learning_focus,omitempty"`
	WorkingHours       string   `json:"working_hours,omitempty"`
	Timezone           string   `json:"timezone,omitempty"`
}

type WorkStyle struct {
	Preferences       []string          `json:"preferences,omitempty"`
	Workflow          string            `json:"workflow,omitempty"`
	DecisionStyle     string            `json:"decision_style,omitempty"`
	FeedbackStyle     string            `json:"feedback_style,omitempty"`
	CollabStyle       string            `json:"collab_style,omitempty"`
	Tools             []string          `json:"tools,omitempty"`
	Languages         []string          `json:"languages,omitempty"`
	DoNotDo           []string          `json:"do_not_do,omitempty"`
	OutputPreferences map[string]string `json:"output_preferences,omitempty"`
}

type Environment struct {
	OS             string   `json:"os,omitempty"`
	Shell          string   `json:"shell,omitempty"`
	Editor         string   `json:"editor,omitempty"`
	Terminal       string   `json:"terminal,omitempty"`
	Hardware       string   `json:"hardware,omitempty"`
	PackageManager string   `json:"package_manager,omitempty"`
	DotfilesRepo   string   `json:"dotfiles_repo,omitempty"`
	KeyTools       []string `json:"key_tools,omitempty"`
	Aliases        []string `json:"aliases,omitempty"`
}

func Load(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &p, nil
}

func Import(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("parse %s: %w", src, err)
	}
	if err := os.MkdirAll(".soul-forge", 0755); err != nil {
		return err
	}
	return writeProfile(dst, &p)
}

func Merge(src, dst string) error {
	incoming, err := func() (*Profile, error) {
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		var p Profile
		return &p, json.Unmarshal(data, &p)
	}()
	if err != nil {
		return fmt.Errorf("read src: %w", err)
	}

	existing := &Profile{}
	if data, err := os.ReadFile(dst); err == nil {
		if err := json.Unmarshal(data, existing); err != nil {
			return fmt.Errorf("parse existing profile %s: %w", dst, err)
		}
	}

	merged := mergeProfiles(existing, incoming)
	if err := os.MkdirAll(".soul-forge", 0755); err != nil {
		return err
	}
	return writeProfile(dst, merged)
}

func mergeProfiles(base, overlay *Profile) *Profile {
	// Identity
	if overlay.Identity.Name != "" {
		base.Identity.Name = overlay.Identity.Name
	}
	if overlay.Identity.Role != "" {
		base.Identity.Role = overlay.Identity.Role
	}
	if overlay.Identity.Background != "" {
		base.Identity.Background = overlay.Identity.Background
	}
	if len(overlay.Identity.Goals) > 0 {
		base.Identity.Goals = overlay.Identity.Goals
	}
	if overlay.Identity.CommunicationStyle != "" {
		base.Identity.CommunicationStyle = overlay.Identity.CommunicationStyle
	}
	if len(overlay.Identity.ExpertiseAreas) > 0 {
		base.Identity.ExpertiseAreas = overlay.Identity.ExpertiseAreas
	}
	if len(overlay.Identity.LearningFocus) > 0 {
		base.Identity.LearningFocus = overlay.Identity.LearningFocus
	}
	if overlay.Identity.WorkingHours != "" {
		base.Identity.WorkingHours = overlay.Identity.WorkingHours
	}
	if overlay.Identity.Timezone != "" {
		base.Identity.Timezone = overlay.Identity.Timezone
	}

	// WorkStyle
	if len(overlay.WorkStyle.Preferences) > 0 {
		base.WorkStyle.Preferences = overlay.WorkStyle.Preferences
	}
	if overlay.WorkStyle.Workflow != "" {
		base.WorkStyle.Workflow = overlay.WorkStyle.Workflow
	}
	if overlay.WorkStyle.DecisionStyle != "" {
		base.WorkStyle.DecisionStyle = overlay.WorkStyle.DecisionStyle
	}
	if overlay.WorkStyle.FeedbackStyle != "" {
		base.WorkStyle.FeedbackStyle = overlay.WorkStyle.FeedbackStyle
	}
	if overlay.WorkStyle.CollabStyle != "" {
		base.WorkStyle.CollabStyle = overlay.WorkStyle.CollabStyle
	}
	if len(overlay.WorkStyle.Tools) > 0 {
		base.WorkStyle.Tools = overlay.WorkStyle.Tools
	}
	if len(overlay.WorkStyle.Languages) > 0 {
		base.WorkStyle.Languages = overlay.WorkStyle.Languages
	}
	if len(overlay.WorkStyle.DoNotDo) > 0 {
		base.WorkStyle.DoNotDo = overlay.WorkStyle.DoNotDo
	}
	if len(overlay.WorkStyle.OutputPreferences) > 0 {
		if base.WorkStyle.OutputPreferences == nil {
			base.WorkStyle.OutputPreferences = make(map[string]string)
		}
		for k, v := range overlay.WorkStyle.OutputPreferences {
			base.WorkStyle.OutputPreferences[k] = v
		}
	}

	// Environment
	if overlay.Environment.OS != "" {
		base.Environment.OS = overlay.Environment.OS
	}
	if overlay.Environment.Shell != "" {
		base.Environment.Shell = overlay.Environment.Shell
	}
	if overlay.Environment.Editor != "" {
		base.Environment.Editor = overlay.Environment.Editor
	}
	if overlay.Environment.Terminal != "" {
		base.Environment.Terminal = overlay.Environment.Terminal
	}
	if overlay.Environment.Hardware != "" {
		base.Environment.Hardware = overlay.Environment.Hardware
	}
	if overlay.Environment.PackageManager != "" {
		base.Environment.PackageManager = overlay.Environment.PackageManager
	}
	if overlay.Environment.DotfilesRepo != "" {
		base.Environment.DotfilesRepo = overlay.Environment.DotfilesRepo
	}
	if len(overlay.Environment.KeyTools) > 0 {
		base.Environment.KeyTools = overlay.Environment.KeyTools
	}
	if len(overlay.Environment.Aliases) > 0 {
		base.Environment.Aliases = overlay.Environment.Aliases
	}

	// UpdatedAt always takes the overlay value if set
	if overlay.UpdatedAt != "" {
		base.UpdatedAt = overlay.UpdatedAt
	}

	return base
}

func writeProfile(path string, p *Profile) error {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
