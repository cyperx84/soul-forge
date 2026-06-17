// Package voice is a deterministic stylometry scanner: it reads a person's writing
// samples and emits measurable signals of their voice (sentence rhythm, punctuation
// tics, hedging, distinctive vocabulary). It mirrors the dotfiles scanner — a no-LLM
// Go pass that writes JSON for the harness to read and confirm with the user.
//
// Crucially it *proposes candidates, never authors the persona*. LLMs can't reliably
// reproduce implicit writing style (arXiv:2509.14543), and surface stats overfit to
// tics — so the deterministic pass surfaces verifiable signals a human confirms, and
// the real payoff is seeding example exchanges, not adjectives.
package voice

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// minReliableWords is the floor below which stylometry is noisy; authorship work
// wants a few thousand words. Below it, Confidence is "low".
const minReliableWords = 2000

type Analysis struct {
	WordCount  int        `json:"word_count"`
	Confidence string     `json:"confidence"` // "low" | "ok"
	Note       string     `json:"note,omitempty"`
	Metrics    Metrics    `json:"metrics"`
	Candidates Candidates `json:"candidates"`
}

type Metrics struct {
	Sentences        int                `json:"sentences"`
	MeanSentenceLen  float64            `json:"mean_sentence_len"`
	PctShortSentence float64            `json:"pct_short_sentences"` // < 8 words
	PctLongSentence  float64            `json:"pct_long_sentences"`  // > 25 words
	MeanWordLen      float64            `json:"mean_word_len"`
	ContractionRate  float64            `json:"contraction_rate"` // per sentence
	FleschReadingEa  float64            `json:"flesch_reading_ease"`
	HedgeRate        float64            `json:"hedge_per_1000w"`
	MTLD             float64            `json:"mtld_lexical_diversity"`
	Punctuation      map[string]float64 `json:"punctuation_per_1000_tokens"`
}

// Candidates are *suggested* persona fields, phrased for the user to confirm/edit.
type Candidates struct {
	Voice      []string `json:"voice"`
	Avoid      []string `json:"avoid"`
	Vocabulary []string `json:"vocabulary"`
}

var contractionMarks = []string{"n't", "'re", "'ve", "'ll", "'m", "'d", "'s"}

var hedgeWords = set("maybe", "might", "perhaps", "possibly", "probably", "seems",
	"somewhat", "arguably", "presumably", "apparently", "kinda", "sorta")

var fillerWords = set("just", "actually", "really", "basically", "literally",
	"honestly", "simply", "very", "quite")

// Analyze runs the full deterministic pass over the concatenated sample text.
func Analyze(text string) *Analysis {
	sentences := splitSentences(text)
	toks := words(text)
	wc := len(toks)

	a := &Analysis{WordCount: wc, Confidence: "ok"}
	if wc < minReliableWords {
		a.Confidence = "low"
		a.Note = "fewer than 2000 words — signals are noisy; treat candidates as weak hints and add more samples."
	}
	if wc == 0 {
		return a
	}

	a.Metrics = measure(text, sentences, toks)
	a.Candidates = candidates(a.Metrics, toks)
	return a
}

func measure(text string, sentences []string, toks []string) Metrics {
	wc := len(toks)
	m := Metrics{Sentences: len(sentences), Punctuation: map[string]float64{}}

	var sentLens []int
	short, long, totalSent := 0, 0, 0
	for _, s := range sentences {
		n := len(words(s))
		if n == 0 {
			continue
		}
		sentLens = append(sentLens, n)
		totalSent += n
		if n < 8 {
			short++
		}
		if n > 25 {
			long++
		}
	}
	if len(sentLens) > 0 {
		m.MeanSentenceLen = round(float64(totalSent) / float64(len(sentLens)))
		m.PctShortSentence = round(100 * float64(short) / float64(len(sentLens)))
		m.PctLongSentence = round(100 * float64(long) / float64(len(sentLens)))
	}

	chars := 0
	for _, w := range toks {
		chars += len([]rune(w))
	}
	m.MeanWordLen = round(float64(chars) / float64(wc))

	contractions := 0
	for _, mark := range contractionMarks {
		contractions += strings.Count(strings.ToLower(text), mark)
	}
	if len(sentLens) > 0 {
		m.ContractionRate = round(float64(contractions) / float64(len(sentLens)))
	}

	m.FleschReadingEa = round(fleschReadingEase(toks, len(sentLens)))

	hedges := 0
	for _, w := range toks {
		if hedgeWords[strings.ToLower(w)] {
			hedges++
		}
	}
	m.HedgeRate = round(1000 * float64(hedges) / float64(wc))
	m.MTLD = round(mtld(lowerAll(toks)))

	// Punctuation per 1000 tokens.
	per1k := func(count int) float64 { return round(1000 * float64(count) / float64(wc)) }
	m.Punctuation["em_dash"] = per1k(strings.Count(text, "—") + strings.Count(text, "--"))
	m.Punctuation["semicolon"] = per1k(strings.Count(text, ";"))
	m.Punctuation["colon"] = per1k(strings.Count(text, ":"))
	m.Punctuation["exclamation"] = per1k(strings.Count(text, "!"))
	m.Punctuation["question"] = per1k(strings.Count(text, "?"))
	m.Punctuation["parenthesis"] = per1k(strings.Count(text, "("))
	m.Punctuation["ellipsis"] = per1k(strings.Count(text, "…") + strings.Count(text, "..."))
	return m
}

func candidates(m Metrics, toks []string) Candidates {
	var c Candidates
	switch {
	case m.MeanSentenceLen > 0 && m.MeanSentenceLen < 12:
		c.Voice = append(c.Voice, fmt.Sprintf("short, declarative sentences (avg %.0f words)", m.MeanSentenceLen))
	case m.MeanSentenceLen > 22:
		c.Voice = append(c.Voice, fmt.Sprintf("long, clause-heavy sentences (avg %.0f words)", m.MeanSentenceLen))
	}
	if m.Punctuation["em_dash"] >= 4 {
		c.Voice = append(c.Voice, "frequent em-dash user")
	}
	if m.ContractionRate >= 0.6 {
		c.Voice = append(c.Voice, "casual, contraction-heavy register")
	} else if m.ContractionRate < 0.15 && m.Sentences > 5 {
		c.Voice = append(c.Voice, "formal register, few contractions")
	}
	if m.FleschReadingEa >= 70 {
		c.Voice = append(c.Voice, "plain, accessible prose")
	} else if m.FleschReadingEa > 0 && m.FleschReadingEa < 40 {
		c.Voice = append(c.Voice, "dense, complex prose")
	}
	if m.HedgeRate >= 8 {
		c.Voice = append(c.Voice, "qualifies claims (hedges often)")
	}

	// Anti-pattern candidates: filler/hedges the user might want the agent to drop.
	if found := presentWords(toks, fillerWords); len(found) > 0 {
		c.Avoid = append(c.Avoid, "filler words: "+strings.Join(found, ", "))
	}
	if m.Punctuation["exclamation"] >= 6 {
		c.Avoid = append(c.Avoid, "frequent exclamation marks (confirm: keep or cut?)")
	}

	c.Vocabulary = distinctiveVocab(toks, 15)
	return c
}

// --- helpers ---

func splitSentences(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == '\n'
	})
}

func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '\'')
	})
}

func fleschReadingEase(toks []string, sentences int) float64 {
	if sentences == 0 || len(toks) == 0 {
		return 0
	}
	syll := 0
	for _, w := range toks {
		syll += syllables(w)
	}
	wps := float64(len(toks)) / float64(sentences)
	spw := float64(syll) / float64(len(toks))
	return 206.835 - 1.015*wps - 84.6*spw
}

// syllables is a vowel-group heuristic (min 1 per word).
func syllables(word string) int {
	w := strings.ToLower(word)
	n, prevVowel := 0, false
	for _, r := range w {
		isVowel := strings.ContainsRune("aeiouy", r)
		if isVowel && !prevVowel {
			n++
		}
		prevVowel = isVowel
	}
	if strings.HasSuffix(w, "e") && n > 1 {
		n--
	}
	if n < 1 {
		n = 1
	}
	return n
}

// mtld — Measure of Textual Lexical Diversity (length-invariant, unlike raw TTR).
func mtld(tokens []string) float64 {
	if len(tokens) < 50 {
		return 0
	}
	return round((mtldForward(tokens) + mtldForward(reverse(tokens))) / 2)
}

func mtldForward(tokens []string) float64 {
	const ttrThreshold = 0.72
	factors, tokenCount := 0.0, 0
	types := map[string]struct{}{}
	for _, t := range tokens {
		tokenCount++
		types[t] = struct{}{}
		ttr := float64(len(types)) / float64(tokenCount)
		if ttr <= ttrThreshold {
			factors++
			types = map[string]struct{}{}
			tokenCount = 0
		}
	}
	if tokenCount > 0 {
		ttr := float64(len(types)) / float64(tokenCount)
		factors += (1 - ttr) / (1 - ttrThreshold)
	}
	if factors == 0 {
		return float64(len(tokens))
	}
	return float64(len(tokens)) / factors
}

func distinctiveVocab(toks []string, max int) []string {
	freq := map[string]int{}
	for _, w := range toks {
		lw := strings.ToLower(w)
		if len(lw) > 4 && !commonWords[lw] {
			freq[lw]++
		}
	}
	type kv struct {
		w string
		n int
	}
	var kvs []kv
	for w, n := range freq {
		if n < 2 { // a one-off isn't a signature term
			continue
		}
		kvs = append(kvs, kv{w, n})
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].n != kvs[j].n {
			return kvs[i].n > kvs[j].n
		}
		return kvs[i].w < kvs[j].w
	})
	var out []string
	for i, e := range kvs {
		if i >= max {
			break
		}
		out = append(out, e.w)
	}
	return out
}

func presentWords(toks []string, want map[string]bool) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, w := range toks {
		lw := strings.ToLower(w)
		if want[lw] {
			if _, ok := seen[lw]; !ok {
				seen[lw] = struct{}{}
				out = append(out, lw)
			}
		}
	}
	sort.Strings(out)
	return out
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

func reverse(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[len(in)-1-i] = s
	}
	return out
}

func round(f float64) float64 { return math.Round(f*100) / 100 }

func set(items ...string) map[string]bool {
	m := make(map[string]bool, len(items))
	for _, it := range items {
		m[it] = true
	}
	return m
}

// commonWords is a small stoplist so distinctive-vocab surfaces content, not glue.
var commonWords = set(
	"about", "above", "after", "again", "their", "there", "these", "those",
	"would", "could", "should", "which", "while", "where", "being", "because",
	"before", "between", "every", "other", "still", "thing", "things", "really",
	"always", "never", "often", "instead", "without", "within", "around", "going",
	"think", "know", "like", "just", "that", "this", "with", "from", "they",
	"them", "then", "than", "what", "when", "your", "have", "here", "more", "some",
)
