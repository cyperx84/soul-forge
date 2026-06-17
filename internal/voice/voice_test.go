package voice

import (
	"strings"
	"testing"
)

func TestAnalyzeBasics(t *testing.T) {
	// Short, punchy, em-dash-heavy, contraction-heavy text.
	text := strings.Repeat("I won't hedge. Ship it — fast. It's the only way. Don't overthink it. ", 20)
	a := Analyze(text)

	if a.WordCount == 0 || a.Metrics.Sentences == 0 {
		t.Fatalf("nothing measured: %+v", a)
	}
	if a.Metrics.MeanSentenceLen >= 12 {
		t.Errorf("expected short sentences, got %.1f", a.Metrics.MeanSentenceLen)
	}
	if a.Metrics.Punctuation["em_dash"] == 0 {
		t.Errorf("expected em-dashes to register")
	}
	if a.Metrics.ContractionRate == 0 {
		t.Errorf("expected contractions to register")
	}
	// Candidate voice should mention short sentences.
	joined := strings.Join(a.Candidates.Voice, " | ")
	if !strings.Contains(joined, "short") {
		t.Errorf("expected a 'short sentences' candidate, got: %s", joined)
	}
}

func TestAnalyzeConfidenceGate(t *testing.T) {
	a := Analyze("Just a few words here.")
	if a.Confidence != "low" || a.Note == "" {
		t.Errorf("short input should be low-confidence with a note: %+v", a)
	}
}

func TestAnalyzeEmpty(t *testing.T) {
	a := Analyze("")
	if a.WordCount != 0 {
		t.Errorf("empty text should have 0 words")
	}
}

func TestMTLDLengthRobustness(t *testing.T) {
	// MTLD should stay in a sane positive range for diverse text (not collapse like TTR).
	toks := strings.Fields(strings.Repeat("the quick brown fox jumps over a lazy dog and runs far away today ", 30))
	got := mtld(toks)
	if got <= 0 {
		t.Errorf("MTLD should be positive for diverse text, got %v", got)
	}
}
