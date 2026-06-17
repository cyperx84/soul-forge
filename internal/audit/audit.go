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

// fileSpec describes a generated file the audit expects to find.
type fileSpec struct {
	name     string
	required bool // required files error when missing; others warn
}

// auditFiles is the set of files `generate` produces, in check order.
var auditFiles = []fileSpec{
	{"SOUL.md", true},
	{"IDENTITY.md", false},
	{"USER.md", true},
	{"AGENTS.md", false},
	{"TOOLS.md", false},
	{"MEMORY.md", false},
}

// vaguePhrases are hedging, generic phrasings that weaken a SOUL file. A great
// soul file is sharp and opinionated; these signal the opposite.
var vaguePhrases = []string{
	"be helpful", "maintain professionalism", "comprehensive and thoughtful",
	"as an ai", "as a language model", "high-quality assistance", "best of my ability",
	// Filler/preamble the role defaults explicitly tell agents to drop.
	"happy to help", "i'd be happy to", "i hope this helps", "great question",
	"let me know if you", "feel free to", "certainly!", "dive deep", "in today's",
}

// soulSections are the persona sections a strong SOUL.md should carry.
var soulSections = []string{"What I Believe", "How I Decide", "What I Won't Do"}

// maxSoulWords is the soft ceiling for SOUL.md; past this it starts diluting itself.
const maxSoulWords = 1500

func Run(cfg *config.Config, agents []config.Agent) []Result {
	profileStat, _ := os.Stat(".soul-forge/profile.json")

	var results []Result
	for _, agent := range agents {
		result := Result{Agent: agent.Name}
		agentDir := filepath.Join(cfg.OutputDir, agent.Name)
		for _, spec := range auditFiles {
			checkFile(agentDir, spec, profileStat, &result)
		}
		results = append(results, result)
	}
	return results
}

func checkFile(agentDir string, spec fileSpec, profileStat os.FileInfo, result *Result) {
	path := filepath.Join(agentDir, spec.name)
	stat, err := os.Stat(path)
	if err != nil {
		sev := "warning"
		if spec.required {
			sev = "error"
		}
		result.Issues = append(result.Issues, Issue{
			Severity: sev,
			File:     spec.name,
			Message:  "file missing — run `soul-forge generate`",
		})
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		result.Issues = append(result.Issues, Issue{
			Severity: "error",
			File:     spec.name,
			Message:  fmt.Sprintf("cannot read: %v", err),
		})
		return
	}
	content := string(data)

	// SOUL.md and USER.md are fully generated, so empty sections or placeholder
	// text there signal a thin profile. AGENTS/TOOLS/MEMORY intentionally ship with
	// TODOs and seed text, so we don't flag those the same way.
	if spec.name == "SOUL.md" || spec.name == "USER.md" {
		for _, s := range checkEmptySections(content) {
			result.Issues = append(result.Issues, Issue{
				Severity: "warning",
				File:     spec.name,
				Message:  fmt.Sprintf("section %q appears empty or has placeholder content", s),
			})
		}
		if strings.Contains(content, "Not specified") || strings.Contains(content, "Not provided") {
			result.Issues = append(result.Issues, Issue{
				Severity: "info",
				File:     spec.name,
				Message:  "contains placeholder values — consider filling out profile.json",
			})
		}
	}

	if spec.name == "SOUL.md" {
		checkSoulQuality(content, result)
	}

	// AGENTS.md and TOOLS.md ship with TODO placeholders for project-specific detail.
	// Nudge (info only) so they don't get forgotten.
	if (spec.name == "AGENTS.md" || spec.name == "TOOLS.md") && strings.Contains(content, "> TODO") {
		result.Issues = append(result.Issues, Issue{
			Severity: "info",
			File:     spec.name,
			Message:  "still has TODO placeholders — fill in project-specific details",
		})
	}

	if profileStat != nil {
		age := time.Since(stat.ModTime())
		profileAge := time.Since(profileStat.ModTime())
		if profileAge < age {
			result.Issues = append(result.Issues, Issue{
				Severity: "warning",
				File:     spec.name,
				Message:  fmt.Sprintf("stale — profile.json is newer (file: %s ago, profile: %s ago)", age.Round(time.Minute), profileAge.Round(time.Minute)),
			})
		}
	}
}

// checkSoulQuality applies soul-file-specific quality heuristics: persona sections
// present, no vague/hedging language, an example exchange, and a sane length.
func checkSoulQuality(content string, result *Result) {
	for _, section := range soulSections {
		if !strings.Contains(content, section) {
			result.Issues = append(result.Issues, Issue{
				Severity: "warning",
				File:     "SOUL.md",
				Message:  fmt.Sprintf("missing persona section %q — a strong soul file states what it believes, how it decides, and what it won't do", section),
			})
		}
	}

	// Counter-examples deliberately contain wrong-voice phrasing — scanning them for
	// vague language would flag the very anti-patterns they're meant to teach against.
	lower := strings.ToLower(dropSection(content, "How I Don't Respond"))
	for _, phrase := range vaguePhrases {
		if strings.Contains(lower, phrase) {
			result.Issues = append(result.Issues, Issue{
				Severity: "warning",
				File:     "SOUL.md",
				Message:  fmt.Sprintf("vague/hedging phrasing %q — sharp, opinionated language makes a better agent", phrase),
			})
		}
	}

	if !strings.Contains(content, "How I Respond") {
		result.Issues = append(result.Issues, Issue{
			Severity: "info",
			File:     "SOUL.md",
			Message:  "no example exchanges — adding 1-2 (persona.examples in soul-forge.yaml), ideally with a counter-example, is the single best way to lock in voice",
		})
	}

	// Predictiveness: the bar for a strong soul is that a reader could predict the
	// agent's take on a *new* topic. Concrete opinions are what make that possible;
	// a thin list reads generic no matter how clean the prose.
	if countBulletsUnder(content, "What I Believe") < 2 {
		result.Issues = append(result.Issues, Issue{
			Severity: "info",
			File:     "SOUL.md",
			Message:  "few concrete opinions — a reader should be able to predict this agent's take on a topic it's never seen; add sharp, specific stances",
		})
	}
	if !strings.Contains(content, "Tensions & Contradictions") {
		result.Issues = append(result.Issues, Issue{
			Severity: "info",
			File:     "SOUL.md",
			Message:  "no Tensions & Contradictions — an all-virtues persona reads generic; one honest contradiction makes it identifiable",
		})
	}

	if n := len(strings.Fields(content)); n > maxSoulWords {
		result.Issues = append(result.Issues, Issue{
			Severity: "warning",
			File:     "SOUL.md",
			Message:  fmt.Sprintf("SOUL.md is %d words (> %d) — long soul files dilute themselves; trim to the essentials", n, maxSoulWords),
		})
	}
}

// dropSection returns content with the "## " section whose heading contains the
// given substring removed (up to the next "## " heading or end). Used to exclude
// intentionally wrong-voice content from quality scans.
func dropSection(content, headingSubstr string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, ln := range lines {
		if strings.HasPrefix(ln, "## ") {
			skipping = strings.Contains(ln, headingSubstr)
		}
		if !skipping {
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
}

// countBulletsUnder counts markdown "- " bullets directly under the given "## "
// heading, stopping at the next "## " heading. Used to gauge persona richness.
func countBulletsUnder(content, heading string) int {
	lines := strings.Split(content, "\n")
	in, n := false, 0
	for _, ln := range lines {
		if strings.HasPrefix(ln, "## ") {
			in = strings.TrimSpace(strings.TrimPrefix(ln, "## ")) == heading
			continue
		}
		if in && strings.HasPrefix(strings.TrimSpace(ln), "- ") {
			n++
		}
	}
	return n
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
