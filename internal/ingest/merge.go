package ingest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

// Merging is where the hand-sync actually dies, and it is the only step that can kill
// it.
//
// The first end-to-end run compiled the real corpus and reproduced the duplication
// instead of collapsing it: every fragment came out harness:openclaw or harness:claude
// and none came out harness:any, because ingest reads files, a file lives under one
// harness directory, and so the harness axis is decided by where a line was found.
// "Install policy: Homebrew first" ranked #2 at a score of 1.000 — byte-identical —
// and both copies survived. Detecting drift and faithfully preserving it is progress,
// but it is not the payoff.
//
// The reason it cannot be automated is the reason Confirm exists. Two lines saying the
// same thing in two files is *evidence of* one shared fragment, never proof:
// TOOLS.md's messaging rule and CLAUDE.md's could legitimately diverge tomorrow, and
// collapsing them on similarity is the silent guess the whole model is built to
// prevent.
//
// **A merge is a widening, not an equality claim, and the widening is the bigger
// claim.** Two lines living under openclaw and claude are evidence for those two
// harnesses. The axis model has no way to say "openclaw and claude but not hermes" —
// a value is one concrete id or `any` — so merging them says `any`, which sends the
// line to Hermes and Codex too. That is strictly more than was observed, and for some
// lines it is flatly wrong: "never use exec/curl for provider messaging — OpenClaw
// routes internally" is true on every box and false for every harness that has no
// internal routing.
//
// So the question a reviewer gets is not "are these the same?" — they can see that
// from the score. It is "should this reach these targets it has never reached?", with
// the newly-reached targets named. That is the decision they are actually making, and
// the honest failure of the earlier design was asking the easy question and performing
// the hard one.

// MergeTarget is a compile target the reviewer's widening would newly reach. The
// caller supplies the fleet's real targets — ingest cannot discover them, and deriving
// them would be the guess this package refuses to make.
type MergeTarget struct {
	Name     string
	Selector fragment.Selector
}

// MergeQuestion asks whether one ranked pair is one fragment or two.
type MergeQuestion struct {
	// Key identifies the question for scripted answers. Stable across runs for the
	// same two lines.
	Key string

	// Pair is the ranked duplicate that raised the question, carrying the score and
	// the shared rare terms that surfaced it. Rank is the contract; the score is not
	// (see Duplicates).
	Pair Pair

	// Widens names each axis the merge would widen to `any`, mapped to the concrete
	// values being given up ("openclaw, claude"). Empty means the two lines already
	// agree on every certain axis and merging widens nothing — the free case.
	Widens map[string]string

	// Blast names the targets the merged fragment would reach that neither line
	// reaches today. This is the actual consequence of answering yes. An empty Blast
	// with a non-empty Widens means no configured target is affected — true today,
	// and not a promise about the next machine added to the fleet.
	Blast []string

	// NeedsText reports that the two lines are worded differently, so a merged
	// fragment has no text until a human writes one. Picking the longer line, or A
	// over B, would be a machine authoring doctrine.
	NeedsText bool

	// NeedsKind reports that the two lines were proposed different kinds. Kind has no
	// `any`, so a merge cannot widen it — the reviewer states which kind survives.
	NeedsKind bool

	// Refused, when non-empty, explains why this pair cannot be merged at all.
	// The question is still emitted: a #1-ranked pair vanishing from the list with no
	// explanation reads as a bug in the ranking.
	Refused string
}

// MergeAnswer is a reviewer's decision on one question.
type MergeAnswer struct {
	Key string

	// Merge is the decision. False, or an absent answer, leaves two fragments.
	Merge bool

	// Text is the merged line, required when NeedsText.
	Text string

	// Kind is the surviving kind, required when NeedsKind.
	Kind string
}

// MergeQuestions turns ranked duplicate pairs into merge questions, most-similar
// first.
//
// targets are the fleet's compile targets, used to compute Blast. Passing none is
// allowed and means the reviewer widens without being shown what it reaches — sound
// only when the caller genuinely has no targets to check against.
func MergeQuestions(pairs []Pair, ps []Proposal, targets []MergeTarget) []MergeQuestion {
	byOrigin := make(map[string]Proposal, len(ps))
	for _, p := range ps {
		byOrigin[p.Candidate.Origin()] = p
	}

	out := make([]MergeQuestion, 0, len(pairs))
	for _, pair := range pairs {
		a, aok := byOrigin[pair.A.Origin()]
		b, bok := byOrigin[pair.B.Origin()]
		if !aok || !bok {
			// A pair whose lines were never proposed is a caller error, not a
			// question. Skipping quietly would hide a mismatched corpus.
			continue
		}
		out = append(out, question(pair, a, b, targets))
	}
	return out
}

func mergeKey(pair Pair) string {
	return pair.A.Origin() + "~" + pair.B.Origin()
}

func question(pair Pair, a, b Proposal, targets []MergeTarget) MergeQuestion {
	q := MergeQuestion{
		Key:       mergeKey(pair),
		Pair:      pair,
		Widens:    map[string]string{},
		NeedsText: a.Line() != b.Line(),
	}

	// Lifecycle is not a scope axis with a wildcard — it is the line between what the
	// compiler owns and what the runtime writes. An authored rule and a line of
	// runtime memory are not one fragment in any sense: merging them either resurrects
	// instance memory into the compiled corpus or drops an authored rule into a file
	// compile never emits. There is no answer to this question, so it is not asked.
	if a.Lifecycle.Value != b.Lifecycle.Value {
		q.Refused = fmt.Sprintf("lifecycle differs (%s vs %s): authored doctrine and runtime-written memory are never one fragment",
			a.Lifecycle.Value, b.Lifecycle.Value)
		return q
	}

	// Kind gets the same treatment as the scope axes, and for the same reason: an
	// uncertain value is a placeholder awaiting judgment, not a claim. CLAUDE.md's
	// lines are proposed kind:rule with Certain=false because the file mixes kinds —
	// reading that placeholder as a disagreement with SOUL.md's decided kind:voice
	// would bill the reviewer for a conflict that no signal ever asserted. Where kind
	// differs *undecidedly*, the merged kind is simply unresolved and review asks once.
	if a.Kind.Certain && b.Kind.Certain && a.Kind.Value != b.Kind.Value {
		q.NeedsKind = true
	}

	for _, ax := range []struct {
		name string
		a, b Tag
	}{
		{"host", a.Host, b.Host},
		{"profile", a.Profile, b.Profile},
		{"harness", a.Harness, b.Harness},
	} {
		// Only a disagreement between two *decided* values is a widening. When either
		// side is uncertain its value is a placeholder awaiting judgment, not a claim
		// being given up — there is nothing yet to widen, and the merged proposal
		// carries the question forward into review.
		if !ax.a.Certain || !ax.b.Certain || ax.a.Value == ax.b.Value {
			continue
		}
		q.Widens[ax.name] = ax.a.Value + ", " + ax.b.Value
	}

	q.Blast = blast(a, b, q.Widens, targets)
	return q
}

// blast names the targets a merged fragment reaches that neither original does.
//
// Only the widened axes are considered. The others are not part of this decision: an
// axis both lines agree on constrains the merged fragment exactly as it constrains
// them, and an uncertain axis has no value to reason from. Isolating the widened axes
// answers precisely the question being asked — what does saying `any` here buy, and
// what does it cost — rather than describing where the fragment ends up, which is
// review's job and then the compiler's.
func blast(a, b Proposal, widens map[string]string, targets []MergeTarget) []string {
	if len(widens) == 0 || len(targets) == 0 {
		return nil
	}
	var out []string
	for _, t := range targets {
		// The merged fragment is `any` on every widened axis, so it reaches every
		// target. Newly reached means neither original did.
		if reaches(a, widens, t) || reaches(b, widens, t) {
			continue
		}
		out = append(out, t.Name)
	}
	sort.Strings(out)
	return out
}

// reaches reports whether p already selects for t considering only the widened axes.
func reaches(p Proposal, widens map[string]string, t MergeTarget) bool {
	for axis := range widens {
		v := p.tag(axis).Value
		if v == fragment.AxisAny {
			continue
		}
		var want string
		switch axis {
		case "host":
			want = t.Selector.Host
		case "profile":
			want = t.Selector.Profile
		case "harness":
			want = t.Selector.Harness
		}
		// profile:user selects universally — the one profile value that is not an
		// agent id. Treating it as a mismatch would report a blast radius for a
		// fragment that already reaches everything.
		if v == fragment.ProfileUser && axis == "profile" {
			continue
		}
		if v != want {
			return false
		}
	}
	return true
}

// Merge collapses proposals the reviewer confirmed are one fragment.
//
// **An unanswered question is a decline, and that is sound here** — which contradicts
// Apply, one file over, where an unanswered batch is an error. The difference is the
// failure direction, not a change of heart. An unanswered batch loses a rule: nothing
// is emitted and nobody is told. An unanswered merge question emits both lines, which
// is exactly today's state on disk — correct, duplicated, and no worse than not
// running the tool. Forcing an answer to all 889 ranked pairs to reach the ~12 real
// ones is how a migration gets abandoned, and abandoned is the default outcome.
//
// The cost of that default is that silence produces no dedup at all, which is the
// bug this step exists to fix. So callers report Declined: a step that quietly does
// nothing looks identical to a step that ran.
func Merge(ps []Proposal, questions []MergeQuestion, answers []MergeAnswer) (*MergeResult, error) {
	byKey := make(map[string]MergeQuestion, len(questions))
	for _, q := range questions {
		byKey[q.Key] = q
	}

	var unknown []string
	accepted := map[string]MergeAnswer{}
	for _, a := range answers {
		q, ok := byKey[a.Key]
		if !ok {
			// Same reasoning as Apply: an answer matching no question means the corpus
			// moved under a saved answer file, and a merge decided about different text
			// would collapse lines nobody looked at.
			unknown = append(unknown, a.Key)
			continue
		}
		if !a.Merge {
			continue
		}
		if q.Refused != "" {
			return nil, fmt.Errorf("merge %s: %s", a.Key, q.Refused)
		}
		if q.NeedsText && strings.TrimSpace(a.Text) == "" {
			return nil, fmt.Errorf("merge %s: the two lines are worded differently — supply the merged text; choosing one for you would be authoring doctrine", a.Key)
		}
		if q.NeedsKind && a.Kind == "" {
			return nil, fmt.Errorf("merge %s: kinds differ (%s vs %s) and kind has no `any` — state which survives",
				a.Key, kindOf(ps, q.Pair.A), kindOf(ps, q.Pair.B))
		}
		accepted[a.Key] = a
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("merge answers for questions that do not exist (corpus changed since they were written?): %s", strings.Join(unknown, ", "))
	}

	uf := newUnionFind()
	for key, a := range accepted {
		q := byKey[key]
		uf.union(q.Pair.A.Origin(), q.Pair.B.Origin())
		_ = a
	}

	// Group members by root, preserving corpus order within a group and group order by
	// first appearance, so output is deterministic and a reviewer reads files the way
	// they wrote them.
	index := map[string]int{}
	for i, p := range ps {
		index[p.Candidate.Origin()] = i
	}
	groups := map[string][]Proposal{}
	var order []string
	for _, p := range ps {
		root := uf.find(p.Candidate.Origin())
		if _, ok := groups[root]; !ok {
			order = append(order, root)
		}
		groups[root] = append(groups[root], p)
	}

	res := &MergeResult{}
	for _, root := range order {
		members := groups[root]
		if len(members) == 1 {
			res.Proposals = append(res.Proposals, members[0])
			continue
		}
		merged, err := collapse(members, byKey, accepted)
		if err != nil {
			return nil, err
		}
		res.Proposals = append(res.Proposals, merged)
		res.Merged = append(res.Merged, MergedInto{ID: merged.Candidate.Origin(), Origins: merged.Origins()})
	}

	for _, q := range questions {
		if q.Refused != "" {
			res.Refused = append(res.Refused, q)
			continue
		}
		if _, ok := accepted[q.Key]; !ok {
			res.Declined = append(res.Declined, q)
		}
	}
	return res, nil
}

// MergeResult records what merging did and, as importantly, what it did not.
type MergeResult struct {
	// Proposals is the reduced set: merged groups collapsed to one, everything else
	// untouched, in corpus order. This is what Batches consumes.
	Proposals []Proposal

	// Merged traces each collapse back to the lines it absorbed.
	Merged []MergedInto

	// Declined are questions answered no or not answered at all. Every one is a line
	// that stays duplicated in the corpus — report the count, or a merge step that
	// nobody answered is indistinguishable from one that ran.
	Declined []MergeQuestion

	// Refused are pairs that cannot be merged whatever the reviewer says.
	Refused []MergeQuestion
}

// MergedInto traces a merged proposal to its member lines.
type MergedInto struct {
	ID      string   `json:"id"`
	Origins []string `json:"origins"`
}

// collapse builds one proposal from a confirmed group.
func collapse(members []Proposal, byKey map[string]MergeQuestion, accepted map[string]MergeAnswer) (Proposal, error) {
	primary := members[0]
	out := Proposal{Candidate: primary.Candidate}
	for _, m := range members[1:] {
		out.Absorbed = append(out.Absorbed, m.Candidate)
	}

	origins := out.Origins()

	// Lifecycle is refused at question time, so a group can only disagree on it via a
	// transitive path — which equality makes impossible. Checked anyway: an invariant
	// that holds by construction today is exactly the one a later refactor breaks
	// silently.
	for _, m := range members[1:] {
		if m.Lifecycle.Value != primary.Lifecycle.Value {
			return Proposal{}, fmt.Errorf("merge group %s: lifecycle disagrees (%s vs %s) — authored doctrine and runtime memory are never one fragment",
				strings.Join(origins, ", "), primary.Lifecycle.Value, m.Lifecycle.Value)
		}
	}
	out.Lifecycle = Tag{primary.Lifecycle.Value, "merged: every member agreed", primary.Lifecycle.Certain}

	// Text. Identical wording merges for free. Divergent wording is authored by the
	// reviewer, and two answers in one group authoring different lines is a conflict
	// with no defensible tiebreak — picking either silently discards a decision a
	// human made.
	texts := map[string]bool{}
	for _, m := range members {
		texts[m.Line()] = true
	}
	if len(texts) > 1 {
		var chosen []string
		for _, a := range accepted {
			if a.Text == "" || !groupHasKey(members, byKey, a.Key) {
				continue
			}
			if !contains(chosen, a.Text) {
				chosen = append(chosen, a.Text)
			}
		}
		if len(chosen) == 0 {
			return Proposal{}, fmt.Errorf("merge group %s: members are worded differently and no answer supplied the merged text", strings.Join(origins, ", "))
		}
		if len(chosen) > 1 {
			sort.Strings(chosen)
			return Proposal{}, fmt.Errorf("merge group %s: answers supplied conflicting merged text (%s) — one group, one line",
				strings.Join(origins, ", "), strings.Join(quoteAll(chosen), " vs "))
		}
		out.MergedText = chosen[0]
	}

	// Kind. No wildcard exists, so two *decided* kinds in conflict are resolved by the
	// reviewer rather than widened. An undecided kind is a placeholder and contributes
	// nothing to the conflict — it carries the question into review, exactly as an
	// undecided scope axis does (see unify).
	kinds := map[string]bool{}
	kindCertain := true
	for _, m := range members {
		if !m.Kind.Certain {
			kindCertain = false
			continue
		}
		kinds[m.Kind.Value] = true
	}
	switch {
	case !kindCertain && len(kinds) <= 1:
		v := primary.Kind.Value
		for k := range kinds {
			v = k // a decided member's value is the better placeholder to show
		}
		out.Kind = Tag{v, "merged " + strings.Join(origins, ", ") + ": a member's kind was never decided, so the merge does not decide it either", false}
	case len(kinds) > 1:
		var chosen []string
		for _, a := range accepted {
			if a.Kind == "" || !groupHasKey(members, byKey, a.Key) {
				continue
			}
			if !contains(chosen, a.Kind) {
				chosen = append(chosen, a.Kind)
			}
		}
		if len(chosen) == 0 {
			return Proposal{}, fmt.Errorf("merge group %s: members were proposed different kinds and no answer stated which survives", strings.Join(origins, ", "))
		}
		if len(chosen) > 1 {
			sort.Strings(chosen)
			return Proposal{}, fmt.Errorf("merge group %s: answers stated conflicting kinds (%s) — one fragment has one kind",
				strings.Join(origins, ", "), strings.Join(chosen, " vs "))
		}
		out.Kind = Tag{chosen[0], "merged: reviewer chose " + chosen[0] + " across " + strings.Join(origins, ", "), true}
	default:
		out.Kind = Tag{primary.Kind.Value, "merged: every member was kind:" + primary.Kind.Value, true}
	}

	for _, axis := range []string{"host", "profile", "harness"} {
		out.setTag(axis, unify(members, axis, origins))
	}

	out.Flags = mergedFlags(members)
	return out, nil
}

// unify computes a merged tag for one scope axis.
//
// Certainty does not survive a merge with an uncertain member. If one line's host was
// decided m4-mini and the other's was never decided, the merged fragment's host is not
// m4-mini — it is unknown, and the difference between those is a machine-specific fact
// pinned to the fleet or a fleet rule pinned to one box. Both are the role-bleed bug
// one axis over. Carrying the question into review costs one question; guessing costs
// a rule that silently vanishes from every other machine.
//
// Two decided values that disagree widen to `any`, because that is what the reviewer
// confirmed and because the model has no way to name a subset.
func unify(members []Proposal, axis string, origins []string) Tag {
	values := map[string]bool{}
	certain := true
	for _, m := range members {
		t := m.tag(axis)
		if !t.Certain {
			certain = false
			continue
		}
		values[t.Value] = true
	}
	if !certain {
		return Tag{fragment.AxisAny, "merged " + strings.Join(origins, ", ") + ": a member's " + axis + " was never decided, so the merge does not decide it either", false}
	}
	if len(values) == 1 {
		for v := range values {
			return Tag{v, "merged: every member was " + axis + ":" + v, true}
		}
	}
	var vs []string
	for v := range values {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	return Tag{fragment.AxisAny, "merged: reviewer confirmed one fragment across " + axis + " " + strings.Join(vs, ", "), true}
}

// mergedFlags unions the members' flags. A flag raised on one line survives the merge:
// a possible secret does not stop being a possible secret because the line it matched
// turned out to live in two files.
func mergedFlags(members []Proposal) []string {
	var out []string
	for _, m := range members {
		for _, f := range m.Flags {
			if !contains(out, f) {
				out = append(out, f)
			}
		}
	}
	return out
}

func allCertain(members []Proposal, axis string) bool {
	for _, m := range members {
		if !m.tag(axis).Certain {
			return false
		}
	}
	return true
}

// groupHasKey reports whether a question key's pair lies inside this group.
func groupHasKey(members []Proposal, byKey map[string]MergeQuestion, key string) bool {
	q, ok := byKey[key]
	if !ok {
		return false
	}
	var a, b bool
	for _, m := range members {
		if m.Candidate.Origin() == q.Pair.A.Origin() {
			a = true
		}
		if m.Candidate.Origin() == q.Pair.B.Origin() {
			b = true
		}
	}
	return a && b
}

func kindOf(ps []Proposal, c Candidate) string {
	for _, p := range ps {
		if p.Candidate.Origin() == c.Origin() {
			return p.Kind.Value
		}
	}
	return "?"
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// setTag writes a tag by axis name, the inverse of tag().
func (p *Proposal) setTag(axis string, t Tag) {
	switch axis {
	case "host":
		p.Host = t
	case "profile":
		p.Profile = t
	case "harness":
		p.Harness = t
	case "lifecycle":
		p.Lifecycle = t
	case "kind":
		p.Kind = t
	}
}

// unionFind groups pairwise merges into whole groups.
//
// Pairwise is how duplicates are found and it is not how they exist. The same install
// line could appear in TOOLS.md, CLAUDE.md, and a Hermes file; that surfaces as three
// pairs, and applying them one at a time would emit overlapping merges — the shared
// member collapsed twice, producing two fragments from three lines that are one. The
// group is the unit for the same reason the fragment is: the pair is a view.
type unionFind struct{ parent map[string]string }

func newUnionFind() *unionFind { return &unionFind{parent: map[string]string{}} }

func (u *unionFind) find(x string) string {
	p, ok := u.parent[x]
	if !ok {
		u.parent[x] = x
		return x
	}
	if p != x {
		u.parent[x] = u.find(p)
	}
	return u.parent[x]
}

func (u *unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	u.parent[rb] = ra
}
