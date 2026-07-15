// Package ingest reverse-compiles existing agent files into proposed fragments.
//
// This is the migration path: nobody starts from zero. Every machine already has
// hand-written guidance files that took years to accrete, and the corpus has to
// begin as those files rather than as a blank profile someone re-types.
//
// The one thing ingest must never do is silently guess. Tagging a line along four
// scope axes is a judgment call — "Klaw orchestrates the fleet" is profile:klaw only
// if you know Klaw is an agent and not the user — and a wrong tag is exactly the bug
// this whole model exists to prevent. So ingest emits Proposals, never Fragments.
// A Proposal cannot become a Fragment except through Confirm: there is no other code
// path, which makes "compiled an unreviewed guess" structurally impossible rather
// than merely discouraged.
package ingest

import (
	"bufio"
	"io"
	"strings"
)

// Candidate is one extracted line of an existing file, before any tag is proposed.
//
// It keeps its origin because provenance is the whole point of the exercise: the
// round-2 review error was a citation that resolved a different question, and the fix
// is that every fragment can be traced back to the exact line it came from.
type Candidate struct {
	// Text is the line's content, stripped of markdown list markers.
	Text string

	// Path is the file it came from, as given to Extract.
	Path string

	// Line is the 1-indexed line number within Path.
	Line int

	// Section is the nearest preceding markdown heading, or "" at top level. It is
	// a tagging signal, not content: a line under "## Red lines" is a rule whatever
	// its wording.
	Section string
}

// Origin renders the candidate's provenance as "path:line", the same form Fragment
// carries in its Source field.
func (c Candidate) Origin() string {
	return c.Path + ":" + itoa(c.Line)
}

// Extract reads a markdown file and returns one Candidate per authored line.
//
// What counts as a line is deliberately narrow. Bullets and paragraphs carry rules
// and facts; headings, code fences, blockquotes, tables, and horizontal rules carry
// structure. Structure is the render map's job — re-ingesting it would import the
// old file layout into the corpus and reintroduce the file-as-owner bug through the
// back door.
//
// Fenced code blocks are skipped wholesale. A line inside a fence is an example, not
// an instruction, and a fenced `rm -rf` ingested as a rule would be a red line
// inverted.
func Extract(path string, r io.Reader) ([]Candidate, error) {
	var out []Candidate
	var section string
	inFence := false

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for n := 1; sc.Scan(); n++ {
		raw := sc.Text()
		trimmed := strings.TrimSpace(raw)

		if isFence(trimmed) {
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" {
			continue
		}
		if h, ok := heading(trimmed); ok {
			section = h
			continue
		}
		if isStructural(trimmed) {
			continue
		}

		text := stripMarker(trimmed)
		if text == "" {
			continue
		}
		out = append(out, Candidate{
			Text:    text,
			Path:    path,
			Line:    n,
			Section: section,
		})
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func isFence(s string) bool {
	return strings.HasPrefix(s, "```") || strings.HasPrefix(s, "~~~")
}

// heading returns the text of an ATX heading, without the hashes.
func heading(s string) (string, bool) {
	if !strings.HasPrefix(s, "#") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimLeft(s, "#")), true
}

// isStructural reports lines that carry layout rather than instruction: table rows,
// horizontal rules, blockquotes, and HTML comments.
func isStructural(s string) bool {
	switch {
	case strings.HasPrefix(s, "|"):
		return true
	case strings.HasPrefix(s, ">"):
		return true
	case strings.HasPrefix(s, "<!--"):
		return true
	case isRule(s):
		return true
	}
	return false
}

// isRule reports a horizontal rule (---, ***, ___), which also covers YAML
// frontmatter delimiters.
func isRule(s string) bool {
	if len(s) < 3 {
		return false
	}
	c := s[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			return false
		}
	}
	return true
}

// stripMarker removes a leading list marker ("- ", "* ", "1. ") so the candidate
// text is the instruction itself. Ordered-list numbering is layout, not content.
func stripMarker(s string) string {
	if len(s) > 1 && (s[0] == '-' || s[0] == '*' || s[0] == '+') && s[1] == ' ' {
		return strings.TrimSpace(s[2:])
	}
	// Ordered list: digits, then "." or ")", then space.
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(s) && (s[i] == '.' || s[i] == ')') && s[i+1] == ' ' {
		return strings.TrimSpace(s[i+2:])
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
