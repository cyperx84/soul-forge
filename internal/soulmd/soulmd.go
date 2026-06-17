// Package soulmd adapts a soul.md-format persona directory (as authored by
// aaronjmars/soul.md: SOUL.md + optional STYLE.md + examples/) onto a soul-forge
// Persona. soul.md describes a *human's* persona in free-form prose; soul-forge
// describes an *agent's* voice and stance. The mapping is deliberately lossy —
// it lifts the parts that correspond (worldview/opinions → opinions, voice →
// voice, boundaries → boundaries, tensions, pet peeves → avoid, examples) and
// ignores the rest (interests, influences, vocabulary). This is the seam that
// lets the two tools compose: author a soul with soul.md, manage a fleet with
// soul-forge.
package soulmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cyperx84/soul-forge/internal/config"
)

// Parse reads the soul.md directory at dir and returns the mapped persona. It
// errors only when SOUL.md is missing or yields no recognizable persona content.
func Parse(dir string) (*config.Persona, error) {
	soulPath := filepath.Join(dir, "SOUL.md")
	soul, err := os.ReadFile(soulPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", soulPath, err)
	}
	bullets := sectionBullets(string(soul))
	p := &config.Persona{
		Backstory:  firstLine(sectionProse(string(soul), "who i am")),
		Opinions:   dedupe(append(append([]string{}, bullets["worldview"]...), bullets["opinions"]...)),
		Tensions:   bullets["tensions & contradictions"],
		Boundaries: bullets["boundaries"],
		Avoid:      bullets["pet peeves"],
	}

	// Voice and extra anti-patterns come from STYLE.md when present.
	if style, err := os.ReadFile(filepath.Join(dir, "STYLE.md")); err == nil {
		p.Voice = firstLine(sectionProse(string(style), "voice principles"))
		sb := sectionBullets(string(style))
		p.Avoid = dedupe(append(append([]string{}, p.Avoid...), sb["anti-patterns"]...))
	}

	p.Examples = parseExamples(filepath.Join(dir, "examples", "good-outputs.md"))
	p.Counters = parseExamples(filepath.Join(dir, "examples", "bad-outputs.md"))

	if !p.HasContent() {
		return nil, fmt.Errorf("no recognizable persona content in %s", soulPath)
	}
	return p, nil
}

// sectionBullets maps each lower-cased "## " heading to the "- " bullet lines
// beneath it (including bullets nested under "### " sub-headings of that section).
func sectionBullets(md string) map[string][]string {
	out := map[string][]string{}
	var cur string
	for _, ln := range strings.Split(md, "\n") {
		if h, ok := heading(ln, "## "); ok {
			cur = strings.ToLower(h)
			continue
		}
		if _, ok := heading(ln, "### "); ok {
			continue // keep accumulating under the enclosing ## section
		}
		if cur == "" {
			continue
		}
		if item, ok := bullet(ln); ok {
			out[cur] = append(out[cur], item)
		}
	}
	return out
}

// sectionProse returns the first non-empty, non-bullet text paragraph under the
// given lower-cased "## " heading.
func sectionProse(md, lowerHeading string) string {
	var in bool
	var para []string
	for _, ln := range strings.Split(md, "\n") {
		if h, ok := heading(ln, "## "); ok {
			if in && len(para) > 0 {
				break
			}
			in = strings.EqualFold(strings.TrimSpace(h), lowerHeading)
			continue
		}
		if !in {
			continue
		}
		t := strings.TrimSpace(ln)
		if t == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		if _, ok := bullet(ln); ok {
			t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ln), "-"))
		}
		para = append(para, t)
	}
	return strings.TrimSpace(strings.Join(para, " "))
}

var calibrationRE = regexp.MustCompile(`(?i)^\s*(calibration|why)\s*:\s*`)

// parseExamples best-effort parses a soul.md examples file. It anchors each entry
// on a "Calibration:" / "Why:" line, taking the preceding bold/heading label as the
// prompt and the quoted or plain body as the response. Returns nil if the file is
// absent or nothing parses — examples are a bonus, never required.
func parseExamples(path string) []config.Exchange {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []config.Exchange
	var label, body []string
	flush := func(note string) {
		resp := strings.TrimSpace(strings.Join(body, " "))
		if resp != "" {
			prompt := strings.TrimSpace(strings.Join(label, " "))
			if prompt == "" {
				prompt = "(in context)"
			}
			out = append(out, config.Exchange{Prompt: prompt, Response: resp, Note: strings.TrimSpace(note)})
		}
		label, body = nil, nil
	}
	for _, ln := range strings.Split(string(data), "\n") {
		if m := calibrationRE.FindString(ln); m != "" {
			flush(ln[len(m):])
			continue
		}
		if h, ok := heading(ln, "### "); ok {
			if len(body) > 0 {
				flush("")
			}
			label = []string{h}
			continue
		}
		t := strings.TrimSpace(ln)
		switch {
		case t == "":
			// blank line ends an unlabeled, uncalibrated block
			if len(body) > 0 && len(label) == 0 {
				flush("")
			}
		case strings.HasPrefix(t, ">"):
			body = append(body, strings.TrimSpace(strings.TrimPrefix(t, ">")))
		case isBoldLabel(t):
			if len(body) > 0 {
				flush("")
			}
			label = []string{strings.Trim(t, "*")}
		default:
			body = append(body, t)
		}
	}
	flush("")
	return out
}

func heading(line, prefix string) (string, bool) {
	if strings.HasPrefix(line, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
	}
	return "", false
}

func bullet(line string) (string, bool) {
	t := strings.TrimSpace(line)
	for _, p := range []string{"- ", "* "} {
		if strings.HasPrefix(t, p) {
			return strings.TrimSpace(t[len(p):]), true
		}
	}
	return "", false
}

func isBoldLabel(t string) bool {
	return strings.HasPrefix(t, "**") && strings.HasSuffix(t, "**") && len(t) > 4
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
