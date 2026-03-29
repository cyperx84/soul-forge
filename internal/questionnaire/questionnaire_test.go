package questionnaire

import (
	"strings"
	"testing"
)

func TestLoadAllAndSinglePart(t *testing.T) {
	parts, err := Load(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 {
		t.Fatalf("parts=%d", len(parts))
	}
	one, err := Load(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != 1 || one[0].Number != 2 {
		t.Fatalf("unexpected part: %+v", one)
	}
	if _, err := Load(4); err == nil {
		t.Fatal("expected bounds error")
	}
}

func TestPartMarkdown(t *testing.T) {
	p := Part{Number: 1, Title: "Title", Subtitle: "Sub", Questions: []Question{{Text: "What?", Hint: "Because"}}}
	md := p.Markdown()
	for _, s := range []string{"# Part 1: Title", "_Sub_", "## Q1.1 — What?", "> Because", "_Your answer:_"} {
		if !strings.Contains(md, s) {
			t.Fatalf("markdown missing %q in %s", s, md)
		}
	}
}

func TestParseQuestions(t *testing.T) {
	content := "## One\n> hint a\n> hint b\n\n## Two\n"
	qs := parseQuestions(3, content)
	if len(qs) != 2 {
		t.Fatalf("len=%d", len(qs))
	}
	if qs[0].ID != "p3_q1" || qs[0].Hint != "hint a hint b" || qs[1].Text != "Two" || qs[0].Type != "text" {
		t.Fatalf("qs=%+v", qs)
	}
}
