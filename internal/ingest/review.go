package ingest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

// Batching exists because the review bill decides whether this migration finishes.
//
// Measured on the real corpus (workspace + CLAUDE.md, 144 lines): 280 per-line
// per-axis decisions, collapsing to 37 when grouped — 7.6x. That is the difference
// between an afternoon and an abandoned tool, and abandoned is the default outcome
// for anything that opens with 280 questions.
//
// The grouping key is (file, section, unresolved-axis-set), and the section is doing
// the real work. A markdown heading is not decoration: "## Red lines" and "## How we
// operate" are the author's own statement that these lines belong together, made
// while writing them. Every line under AGENTS.md's "How we operate" has the same
// profile answer, because that is what putting them under one heading meant.
//
// The batch never decides. It asks once instead of ten times, and one wrong answer
// now mis-tags ten lines instead of one — so Apply reports exactly what it touched,
// and a batch is always expandable into its members.

// Batch is a set of proposals sharing a file, a section, and the same unresolved
// axes: one question, one answer, applied to every member.
type Batch struct {
	// Path and Section are the group's origin. Section is "" for lines above the
	// first heading.
	Path    string
	Section string

	// Axes are the unresolved axis names shared by every member, sorted.
	Axes []string

	// Members are the proposals in this batch, in corpus order.
	Members []Proposal
}

// Key identifies the batch for display and for scripted answers.
func (b Batch) Key() string {
	s := b.Section
	if s == "" {
		s = "(top)"
	}
	return b.Path + "#" + s + "#" + strings.Join(b.Axes, "+")
}

// Reasons returns one reason per unresolved axis, deduplicated. Members share an axis
// set but can differ in *why* an axis is unresolved — a line naming an agent and a
// line naming none both land in profile-unresolved for different reasons, and a
// reviewer needs both to answer honestly.
func (b Batch) Reasons() map[string][]string {
	out := map[string][]string{}
	for _, axis := range b.Axes {
		seen := map[string]bool{}
		for _, m := range b.Members {
			r := m.tag(axis).Reason
			if r == "" || seen[r] {
				continue
			}
			seen[r] = true
			out[axis] = append(out[axis], r)
		}
		sort.Strings(out[axis])
	}
	return out
}

// tag returns the Tag for an axis name, or the zero Tag for an unknown name.
func (p Proposal) tag(axis string) Tag {
	switch axis {
	case "host":
		return p.Host
	case "profile":
		return p.Profile
	case "harness":
		return p.Harness
	case "lifecycle":
		return p.Lifecycle
	case "kind":
		return p.Kind
	}
	return Tag{}
}

// Batches groups proposals that need the same decision.
//
// Proposals with nothing unresolved are excluded: they are not questions, and padding
// a review queue with items that need no answer trains a reviewer to skim, which is
// how the one item that mattered gets waved through.
//
// Order is by file then first appearance, so a reviewer walks each file top to bottom
// the way they wrote it. Deterministic: same input, same order.
func Batches(ps []Proposal) []Batch {
	type key struct {
		path, section, axes string
	}
	index := map[key]int{}
	var out []Batch

	for _, p := range ps {
		axes := p.Unresolved()
		if len(axes) == 0 {
			continue
		}
		k := key{p.Candidate.Path, p.Candidate.Section, strings.Join(axes, "+")}
		if pos, ok := index[k]; ok {
			out[pos].Members = append(out[pos].Members, p)
			continue
		}
		index[k] = len(out)
		out = append(out, Batch{
			Path:    p.Candidate.Path,
			Section: p.Candidate.Section,
			Axes:    axes,
			Members: []Proposal{p},
		})
	}
	return out
}

// Bill is what a reviewer is being asked for, measured rather than estimated.
//
// It is reported before the queue because the honest number is what lets someone
// decide to start. "37 questions" gets answered; "280 decisions" gets closed.
type Bill struct {
	// Lines is every candidate ingested.
	Lines int
	// Resolved is lines needing no judgment at all.
	Resolved int
	// PerAxis is the unbatched cost: one decision per unresolved axis per line.
	PerAxis int
	// Batched is the batched cost: one decision per batch.
	Batched int
}

// Collapse is how much batching saves. 1.0 means batching bought nothing — which is
// itself worth seeing, because it would mean the sections are not grouping like
// lines and the key is wrong.
func (b Bill) Collapse() float64 {
	if b.Batched == 0 {
		return 1
	}
	return float64(b.PerAxis) / float64(b.Batched)
}

// Measure computes the bill for a set of proposals.
func Measure(ps []Proposal) Bill {
	bill := Bill{Lines: len(ps)}
	for _, p := range ps {
		u := p.Unresolved()
		if len(u) == 0 {
			bill.Resolved++
		}
		bill.PerAxis += len(u)
	}
	bill.Batched = len(Batches(ps))
	return bill
}

// Answer is a reviewer's decision for one batch: a value per unresolved axis.
type Answer struct {
	// Key is the Batch.Key it answers.
	Key string
	// Values maps axis name to the confirmed value.
	Values map[string]string
	// Skip abandons the batch: its members produce no fragments. A reviewer who
	// cannot answer must be able to say so — forcing a value would manufacture
	// exactly the guess Confirm exists to prevent.
	Skip bool
}

// IDFunc names a fragment from its proposal. Ingest does not generate IDs itself: an
// ID is a name, names are a human's job, and a generated one ("agents-md-line-31")
// would encode the file layout back into the corpus — the file-as-owner bug returning
// through the back door.
type IDFunc func(p Proposal, batch Batch, indexInBatch int) string

// Apply turns answered batches into fragments.
//
// Every batch must be answered or skipped. An unanswered batch is an error rather
// than a skip, because "I didn't get to it" and "I decided to drop it" are different
// states, and silently treating the first as the second loses rules without saying so
// — the same class as blurring changed with verified.
//
// Answers apply to every member, and Apply returns what it touched so a wrong answer
// is visible rather than diffused across ten lines.
func Apply(batches []Batch, answers []Answer, id IDFunc) (*ApplyResult, error) {
	byKey := make(map[string]Answer, len(answers))
	for _, a := range answers {
		byKey[a.Key] = a
	}

	res := &ApplyResult{}
	for _, b := range batches {
		a, ok := byKey[b.Key()]
		if !ok {
			return nil, fmt.Errorf("batch %s: unanswered — answer it or skip it explicitly; an unanswered question is not a decision", b.Key())
		}
		if a.Skip {
			res.Skipped = append(res.Skipped, b)
			continue
		}
		for i, m := range b.Members {
			f, err := m.Confirm(id(m, b, i), a.Values)
			if err != nil {
				return nil, fmt.Errorf("batch %s, %s: %w", b.Key(), m.Candidate.Origin(), err)
			}
			res.Fragments = append(res.Fragments, f)
			res.Applied = append(res.Applied, AppliedTo{Batch: b.Key(), Origin: m.Candidate.Origin(), ID: f.ID})
		}
	}

	// Unknown answers are an error, not noise. An answer whose key matches no batch
	// means the corpus moved under a saved answer file, and applying the rest while
	// dropping that one would tag lines from a stale decision.
	keys := map[string]bool{}
	for _, b := range batches {
		keys[b.Key()] = true
	}
	var unknown []string
	for _, a := range answers {
		if !keys[a.Key] {
			unknown = append(unknown, a.Key)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("answers for batches that do not exist (corpus changed since they were written?): %s", strings.Join(unknown, ", "))
	}
	return res, nil
}

// ApplyResult records what Apply did.
type ApplyResult struct {
	Fragments []fragment.Fragment
	Applied   []AppliedTo
	Skipped   []Batch
}

// AppliedTo traces one fragment back to the batch answer that tagged it, so a wrong
// batch answer can be found and reversed rather than hunted line by line.
type AppliedTo struct {
	Batch  string
	Origin string
	ID     string
}
