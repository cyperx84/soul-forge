package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cyperx84/soul-forge/internal/config"
)

type Result struct {
	Agent  string
	Issues []Issue
}

type Issue struct {
	Severity string // error, warning, info
	File     string
	Message  string
}

func (r *Result) HasIssues() bool {
	for _, i := range r.Issues {
		if i.Severity == "error" || i.Severity == "warning" {
			return true
		}
	}
	return false
}

func (r *Result) Format() string {
	var b strings.Builder
	if len(r.Issues) == 0 {
		fmt.Fprintf(&b, "✓ [%s] All checks passed\n", r.Agent)
		return b.String()
	}
	fmt.Fprintf(&b, "▶ [%s]\n", r.Agent)
	for _, issue := range r.Issues {
		icon := "⚠"
		if issue.Severity == "error" {
			icon = "✗"
		} else if issue.Severity == "info" {
			icon = "ℹ"
		}
		if issue.File != "" {
			fmt.Fprintf(&b, "  %s [%s] %s: %s\n", icon, issue.Severity, issue.File, issue.Message)
		} else {
			fmt.Fprintf(&b, "  %s [%s] %s\n", icon, issue.Severity, issue.Message)
		}
	}
	return b.String()
}

func Run(cfg *config.Config, agents []config.Agent) []Result {
	profileStat, _ := os.Stat(".soul-forge/profile.json")

	var results []Result
	for _, agent := range agents {
		result := Result{Agent: agent.Name}
		agentDir := filepath.Join(cfg.OutputDir, agent.Name)

		checkFile(agentDir, "USER.md", profileStat, &result)
		checkFile(agentDir, "SOUL.md", profileStat, &result)

		results = append(results, result)
	}
	return results
}

func checkFile(agentDir, filename string, profileStat os.FileInfo, result *Result) {
	path := filepath.Join(agentDir, filename)
	stat, err := os.Stat(path)
	if err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: "error",
			File:     filename,
			Message:  "file missing — run `soul-forge generate`",
		})
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: "error",
			File:     filename,
			Message:  fmt.Sprintf("cannot read: %v", err),
		})
		return
	}

	content := string(data)
	emptySections := checkEmptySections(content)
	for _, s := range emptySections {
		result.Issues = append(result.Issues, Issue{
			Severity: "warning",
			File:     filename,
			Message:  fmt.Sprintf("section %q appears empty or has placeholder content", s),
		})
	}

	if strings.Contains(content, "Not specified") || strings.Contains(content, "Not provided") {
		result.Issues = append(result.Issues, Issue{
			Severity: "info",
			File:     filename,
			Message:  "contains placeholder values — consider filling out profile.json",
		})
	}

	if profileStat != nil {
		age := time.Since(stat.ModTime())
		profileAge := time.Since(profileStat.ModTime())
		if profileAge < age {
			result.Issues = append(result.Issues, Issue{
				Severity: "warning",
				File:     filename,
				Message:  fmt.Sprintf("stale — profile.json is newer (file: %s ago, profile: %s ago)", age.Round(time.Minute), profileAge.Round(time.Minute)),
			})
		}
	}
}

func checkEmptySections(content string) []string {
	var empty []string
	lines := strings.Split(content, "\n")
	var currentSection string
	sectionContent := false
	for i, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if currentSection != "" && !sectionContent {
				empty = append(empty, currentSection)
			}
			currentSection = strings.TrimPrefix(line, "## ")
			sectionContent = false
			continue
		}
		if currentSection != "" && i > 0 {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && trimmed != "---" && !strings.HasPrefix(trimmed, "_") {
				sectionContent = true
			}
		}
	}
	if currentSection != "" && !sectionContent {
		empty = append(empty, currentSection)
	}
	return empty
}
