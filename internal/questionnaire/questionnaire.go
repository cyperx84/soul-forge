package questionnaire

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed parts
var partsFS embed.FS

type Question struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Hint string `json:"hint,omitempty"`
	Type string `json:"type"` // text, list, choice
}

type Part struct {
	Number    int        `json:"part"`
	Title     string     `json:"title"`
	Subtitle  string     `json:"subtitle"`
	Questions []Question `json:"questions"`
}

func (p *Part) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Part %d: %s\n\n", p.Number, p.Title)
	if p.Subtitle != "" {
		fmt.Fprintf(&b, "_%s_\n\n", p.Subtitle)
	}
	fmt.Fprintf(&b, "---\n\n")
	for i, q := range p.Questions {
		fmt.Fprintf(&b, "## Q%d.%d — %s\n\n", p.Number, i+1, q.Text)
		if q.Hint != "" {
			fmt.Fprintf(&b, "> %s\n\n", q.Hint)
		}
		fmt.Fprintf(&b, "_Your answer:_\n\n\n\n")
	}
	return b.String()
}

func Load(part int) ([]Part, error) {
	allParts, err := loadAll()
	if err != nil {
		return nil, err
	}
	if part == 0 {
		return allParts, nil
	}
	if part < 1 || part > len(allParts) {
		return nil, fmt.Errorf("part must be 1-%d, got %d", len(allParts), part)
	}
	return []Part{allParts[part-1]}, nil
}

func loadAll() ([]Part, error) {
	p1, err := loadPart(1, "parts/part1.md", "Who Are You", "Help your agents understand you as a person — your background, goals, and how you think.")
	if err != nil {
		return nil, err
	}
	p2, err := loadPart(2, "parts/part2.md", "How Should I Work", "Define your working preferences, communication style, and what great collaboration looks like for you.")
	if err != nil {
		return nil, err
	}
	p3, err := loadPart(3, "parts/part3.md", "Your Environment", "Your hardware, OS, tools, and setup — so agents can give you environment-aware advice.")
	if err != nil {
		return nil, err
	}
	return []Part{p1, p2, p3}, nil
}

func loadPart(number int, path, title, subtitle string) (Part, error) {
	data, err := partsFS.ReadFile(path)
	if err != nil {
		return Part{}, fmt.Errorf("load questionnaire part %d (%s): %w", number, path, err)
	}
	return Part{
		Number:    number,
		Title:     title,
		Subtitle:  subtitle,
		Questions: parseQuestions(number, string(data)),
	}, nil
}

// parseQuestions parses a simple format:
// ## Q: <question text>
// > <hint>
func parseQuestions(partNumber int, content string) []Question {
	var questions []Question
	lines := strings.Split(content, "\n")
	var current *Question
	qIndex := 0
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "## ") {
			if current != nil {
				questions = append(questions, *current)
			}
			qIndex++
			text := strings.TrimPrefix(line, "## ")
			current = &Question{
				ID:   fmt.Sprintf("p%d_q%d", partNumber, qIndex),
				Text: text,
				Type: "text",
			}
		} else if strings.HasPrefix(line, "> ") && current != nil {
			hint := strings.TrimPrefix(line, "> ")
			if current.Hint != "" {
				current.Hint += " " + hint
			} else {
				current.Hint = hint
			}
		}
	}
	if current != nil {
		questions = append(questions, *current)
	}
	return questions
}
