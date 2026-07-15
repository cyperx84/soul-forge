package ingest

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

// Pair is two candidate lines that may say the same thing in different words.
//
// It is a candidate for review, never a verdict. The spec forbids an LLM in the CLI,
// and deciding that "don't blur done with attempted" and "don't claim done until
// verified" are one rule is a semantic judgment no regex makes. So the split is the
// same one Propose/Confirm uses: the CLI narrows tens of thousands of pairs to a
// handful mechanically, and the harness or the human rules on those.
//
// Pretending otherwise is the failure this project keeps re-teaching — a green signal
// that is not measuring what you think it is.
type Pair struct {
	A, B Candidate

	// Score is IDF-weighted cosine similarity in [0,1]. It measures shared rare
	// wording, which is a proxy for shared meaning and not the thing itself.
	Score float64

	// Shared lists the rare terms both lines carry, so a reviewer can see *why* the
	// pair surfaced rather than trusting the number.
	Shared []string

	// SameFile reports whether both lines came from one file. Cross-file pairs are
	// hand-sync drift; same-file pairs are a rule restated within its own owner —
	// both are duplication, and the hand review found one of each.
	SameFile bool
}

// FloorDefault drops pairs sharing nothing but a coincidental common word, so a
// reviewer's list stays skimmable. It is deliberately far below any score a real
// duplicate has scored — its job is to bound the list length, not to judge
// similarity. Raising it to "improve precision" is how the done-vs-attempted pair
// got cut once already.
const FloorDefault = 0.05

// wordRe splits on anything that is not a letter, digit, or internal hyphen.
var wordRe = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9'-]*`)

// stopwords are terms carrying no discriminating signal. Kept deliberately short:
// over-pruning throws away the rare-term overlap the whole method depends on, and a
// term that appears everywhere is already scored to near-zero by IDF. This list only
// needs to catch what IDF cannot — words rare in *this* corpus but meaningless.
var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
	"is": true, "are": true, "was": true, "be": true, "been": true, "being": true,
	"to": true, "of": true, "in": true, "on": true, "at": true, "by": true,
	"for": true, "with": true, "from": true, "as": true, "it": true, "its": true,
	"this": true, "that": true, "these": true, "those": true, "you": true,
	"your": true, "not": true, "no": true, "do": true, "does": true, "don't": true,
	"can": true, "will": true, "when": true, "if": true, "then": true, "than": true,
	"so": true, "up": true, "out": true, "into": true, "over": true, "only": true,
	"any": true, "every": true, "all": true, "one": true, "what": true, "which": true,
	"has": true, "have": true, "had": true, "he": true, "his": true, "they": true,
}

// terms normalizes a line into its content terms: lowercased, stopword-free, and
// suffix-stripped so "diagrams" and "diagram" are one term.
func terms(s string) []string {
	var out []string
	for _, w := range wordRe.FindAllString(strings.ToLower(s), -1) {
		if stopwords[w] || len(w) < 3 {
			continue
		}
		out = append(out, stem(w))
	}
	return out
}

// stem strips common inflectional suffixes. Deliberately crude: a full Porter stemmer
// buys accuracy this method does not need, because the output is a ranked shortlist a
// human reads, not a decision. Wrong stems cost a reviewer one glance; a dependency
// costs forever.
func stem(w string) string {
	for _, suf := range []string{"ing", "edly", "ed", "ly", "es", "s"} {
		if len(w) > len(suf)+2 && strings.HasSuffix(w, suf) {
			return strings.TrimSuffix(w, suf)
		}
	}
	return w
}

// idf returns each term's inverse document frequency across the candidate set.
//
// This is what makes the method work at all. Plain word overlap ranks by shared
// *common* words, so every pair of rules scores high on "the agent should" and the
// real duplicates drown. IDF inverts it: two lines both saying "diagram" is
// remarkable precisely because almost nothing else does.
func idf(docs [][]string) map[string]float64 {
	df := map[string]int{}
	for _, d := range docs {
		for t := range set(d) {
			df[t]++
		}
	}
	n := float64(len(docs))
	out := make(map[string]float64, len(df))
	for t, f := range df {
		// +1 smoothing keeps a term appearing in every doc at a small positive
		// weight rather than exactly zero, so an all-common-words pair still scores
		// above an unrelated one instead of both landing at 0.
		out[t] = math.Log(1 + n/float64(f))
	}
	return out
}

func set(ts []string) map[string]bool {
	m := make(map[string]bool, len(ts))
	for _, t := range ts {
		m[t] = true
	}
	return m
}

// Duplicates returns candidate pairs ordered most-similar first, dropping anything
// below floor.
//
// Scoring is cosine similarity over IDF-weighted term sets. Term *sets*, not counts:
// these are one-line rules, so a repeated word is emphasis, not topicality, and
// counting it twice would rank a line's rhetoric over its content.
//
// **Rank is the contract. The score is not.**
//
// Score is corpus-relative and cannot be compared across runs. IDF is computed from
// the candidate set, so the same two lines score differently depending on what else
// was ingested alongside them — ingesting one file or ten changes every number. The
// first version of this package took a `threshold` and defaulted it to 0.15, which
// silently cut the real "done vs attempted" duplicate at 0.137 while keeping noise
// from a larger corpus; the pair was ranked correctly at #3 the whole time and the
// constant threw it away. An absolute cutoff on a relative number is a green tick
// that measures nothing.
//
// So: read the ranked list top-down and stop when the pairs stop being real. floor
// exists only to drop the long tail of one-common-word coincidences that would
// otherwise make the list O(n²) to skim — it is a performance bound, not a
// similarity judgment, and it defaults low for that reason (see FloorDefault).
// Anything built on "score > X means duplicate" is measuring corpus size.
func Duplicates(cands []Candidate, floor float64) []Pair {
	threshold := floor
	docs := make([][]string, len(cands))
	for i, c := range cands {
		docs[i] = terms(c.Text)
	}
	weights := idf(docs)

	// Precompute each line's IDF-weighted term set and its vector norm.
	sets := make([]map[string]bool, len(cands))
	norms := make([]float64, len(cands))
	for i, d := range docs {
		sets[i] = set(d)
		var sum float64
		for t := range sets[i] {
			w := weights[t]
			sum += w * w
		}
		norms[i] = math.Sqrt(sum)
	}

	var out []Pair
	for i := 0; i < len(cands); i++ {
		if norms[i] == 0 {
			continue // no content terms: nothing to compare
		}
		for j := i + 1; j < len(cands); j++ {
			if norms[j] == 0 {
				continue
			}
			var dot float64
			var shared []string
			for t := range sets[i] {
				if sets[j][t] {
					w := weights[t]
					dot += w * w
					shared = append(shared, t)
				}
			}
			if dot == 0 {
				continue
			}
			score := dot / (norms[i] * norms[j])
			if score < threshold {
				continue
			}
			sort.Strings(shared)
			out = append(out, Pair{
				A: cands[i], B: cands[j], Score: score, Shared: shared,
				SameFile: cands[i].Path == cands[j].Path,
			})
		}
	}

	// Deterministic order: score desc, then origin, so output is stable across runs
	// and diffable in review.
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Score != out[b].Score {
			return out[a].Score > out[b].Score
		}
		if out[a].A.Origin() != out[b].A.Origin() {
			return out[a].A.Origin() < out[b].A.Origin()
		}
		return out[a].B.Origin() < out[b].B.Origin()
	})
	return out
}
