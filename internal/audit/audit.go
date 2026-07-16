// Package audit lints a fragment corpus for the problems that are real but not
// build-stopping.
//
// The line between here and compile's invariants is failure cost: an invariant
// encodes a bug that corrupts output (role bleed, secrets, gutted files), so it
// stops the build; an audit finding encodes rot (duplicates, missing provenance,
// pinned project state), so it reports and lets you ship. Promoting a finding to an
// invariant is a one-way door — compile has no override flag by design — which is
// why audit exists as a separate, advisory pass.
package audit

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/cyperx84/soul-forge/internal/compile"
	"github.com/cyperx84/soul-forge/internal/fragment"
)

// Severity orders findings. Warn means "fix before this rots something"; Info means
// "a human should glance at this once".
const (
	SeverityWarn = "warn"
	SeverityInfo = "info"
)

// Finding is one audit result. Check is a stable kebab-case id so CI can filter on
// it; Detail says what to do, not just what is wrong.
type Finding struct {
	Check      string
	Severity   string
	FragmentID string
	Detail     string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s: %s: %s: %s", f.Severity, f.Check, f.FragmentID, f.Detail)
}

// Options selects which passes run and what they run against.
type Options struct {
	// Targets, when non-empty, enables the passes that need compile destinations:
	// narrow-scope candidates and byte budgets.
	Targets []compile.Target

	// Provenance enables the missing-provenance pass. Flag-gated per spec: a corpus
	// authored fresh has no doc citations yet, and drowning it in provenance noise
	// on day one teaches people to ignore the audit.
	Provenance bool

	// BudgetBytes is the per-file budget the bloat pass warns against. Zero uses
	// DefaultBudgetBytes.
	BudgetBytes int

	// Overrides are the child-replaces-parent records from Corpus.Resolve. Reported
	// as info: an override is legitimate, but a downstream box silently changing
	// doctrine is how surprises get provisioned.
	Overrides []fragment.Override
}

// DefaultBudgetBytes is OpenClaw's documented per-file budget. These files inject
// every session, so every byte is a per-session tax.
const DefaultBudgetBytes = 20000

// bloatWarnRatio is where the budget pass starts warning. Warning only at 100%
// would mean the first warning is also the build failure.
const bloatWarnRatio = 0.8

// Run executes every enabled pass over corpus and returns findings sorted by
// severity (warn first), then check, then fragment id — a stable order, because
// audit output lands in CI logs and diffs of it should mean something.
func Run(corpus []fragment.Fragment, opts Options) []Finding {
	var findings []Finding

	findings = append(findings, duplicateText(corpus)...)
	findings = append(findings, projectState(corpus)...)
	findings = append(findings, vagueLanguage(corpus)...)
	if opts.Provenance {
		findings = append(findings, missingProvenance(corpus)...)
	}
	if len(opts.Targets) > 0 {
		findings = append(findings, narrowScope(corpus, opts.Targets)...)
		findings = append(findings, bloat(corpus, opts.Targets, opts.budget())...)
	}
	for _, o := range opts.Overrides {
		findings = append(findings, Finding{
			Check: "override", Severity: SeverityInfo, FragmentID: o.ID,
			Detail: fmt.Sprintf("profile %q overrides its parent's definition; was %q, now %q", o.ChildProfile, truncate(o.Parent.Text), truncate(o.Child.Text)),
		})
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity == SeverityWarn // warn before info
		}
		if findings[i].Check != findings[j].Check {
			return findings[i].Check < findings[j].Check
		}
		return findings[i].FragmentID < findings[j].FragmentID
	})
	return findings
}

func (o Options) budget() int {
	if o.BudgetBytes > 0 {
		return o.BudgetBytes
	}
	return DefaultBudgetBytes
}

// HasWarnings reports whether any finding is warn-severity — the CI exit signal.
func HasWarnings(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == SeverityWarn {
			return true
		}
	}
	return false
}

// duplicateText flags fragments whose normalized text is identical under different
// IDs. Duplicate *fragments*, never duplicate strings-in-output: two targets sharing
// a rendered line is a correct render of one fragment, but two fragments carrying
// the same sentence is the hand-sync surviving inside the corpus — the thing merge
// exists to collapse.
func duplicateText(corpus []fragment.Fragment) []Finding {
	byText := map[string][]string{}
	for _, f := range corpus {
		key := normalize(f.Text)
		if key == "" {
			continue
		}
		byText[key] = append(byText[key], f.ID)
	}
	var findings []Finding
	for _, ids := range byText {
		if len(ids) < 2 {
			continue
		}
		sort.Strings(ids)
		for _, id := range ids {
			findings = append(findings, Finding{
				Check: "duplicate-fragment", Severity: SeverityWarn, FragmentID: id,
				Detail: fmt.Sprintf("same text as %s: one rule, one fragment — run merge, or delete all but one", strings.Join(others(ids, id), ", ")),
			})
		}
	}
	return findings
}

// normalize strips the differences that hide a duplicate without changing meaning:
// case, whitespace runs, and markdown emphasis. Emphasis specifically because the
// real corpus had `**Install policy:**` and `Install policy:` score 1.000 in
// ingest's ranking while remaining byte-distinct. Backticks are kept — code spans
// change meaning.
func normalize(text string) string {
	s := strings.ToLower(strings.TrimSpace(text))
	s = strings.NewReplacer("*", "", "_", "").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func others(ids []string, self string) []string {
	var out []string
	for _, id := range ids {
		if id != self {
			out = append(out, id)
		}
	}
	return out
}

// projectStatePatterns are the shapes current-project state takes when it gets
// pinned into an always-injected file, where it rots. Dates are the strongest
// signal: doctrine is dateless, status is dated.
var projectStatePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b20\d\d-\d\d-\d\d\b`),
	regexp.MustCompile(`(?i)\b(roadmap|in progress|this week|still pending|not yet (done|built|fixed)|TODO:)\b`),
}

// projectState flags authored fragments carrying current-project status. Every
// compiled file injects every session; state pinned there is stale the week after
// it was written and trusted anyway.
func projectState(corpus []fragment.Fragment) []Finding {
	var findings []Finding
	for _, f := range corpus {
		if f.Lifecycle != fragment.LifecycleAuthored {
			continue
		}
		for _, re := range projectStatePatterns {
			if m := re.FindString(f.Text); m != "" {
				findings = append(findings, Finding{
					Check: "project-state", Severity: SeverityWarn, FragmentID: f.ID,
					Detail: fmt.Sprintf("contains %q: current-project state rots in always-injected files — look it up at runtime instead", m),
				})
				break
			}
		}
	}
	return findings
}

// vaguePhrases is v1's vague-language lint, kept small. Each phrase delegates the
// actual decision back to the agent, which is what a rule exists to not do.
var vaguePhrases = []string{
	"as appropriate", "when appropriate", "if necessary", "when necessary",
	"as needed", "and so on", "etc.",
}

func vagueLanguage(corpus []fragment.Fragment) []Finding {
	var findings []Finding
	for _, f := range corpus {
		if f.Kind != fragment.KindRule {
			continue // a fact or voice line saying "etc." is style, not a hole in doctrine
		}
		lower := strings.ToLower(f.Text)
		for _, p := range vaguePhrases {
			if strings.Contains(lower, p) {
				findings = append(findings, Finding{
					Check: "vague-language", Severity: SeverityInfo, FragmentID: f.ID,
					Detail: fmt.Sprintf("rule contains %q: says decide-for-yourself where a rule should decide", p),
				})
				break
			}
		}
	}
	return findings
}

// harnessBehaviorMarkers are the words a fragment uses when it asserts how a harness
// behaves — the class of claim that produced the round-2 review error, where a
// citation resolving a different question laundered a wrong call as confirmed.
// Assertions like these are exactly the ones that need a source line to be
// checkable rather than re-derivable.
var harnessBehaviorMarkers = []string{
	"inject", "sub-agent", "subagent", "prompt-cache", "system prompt",
	"runtime provides", "runtime injects", "at run time", "bootstrap",
}

func missingProvenance(corpus []fragment.Fragment) []Finding {
	var findings []Finding
	for _, f := range corpus {
		if f.Source != "" {
			continue
		}
		lower := strings.ToLower(f.Text)
		for _, m := range harnessBehaviorMarkers {
			if strings.Contains(lower, m) {
				findings = append(findings, Finding{
					Check: "missing-provenance", Severity: SeverityWarn, FragmentID: f.ID,
					Detail: fmt.Sprintf("asserts harness behavior (%q) with no source: citation: unverifiable claims get re-derived or trusted, both badly", m),
				})
				break
			}
		}
	}
	return findings
}

// narrowScope flags fragments carrying an `any` axis that only ever select into one
// of the declared targets. An `any` that reaches one place is a claim of universality
// nothing tests — narrow it and the tag says what the corpus does.
func narrowScope(corpus []fragment.Fragment, targets []compile.Target) []Finding {
	if len(targets) < 2 {
		return nil // with one target, every fragment selects into "only" one — the pass would flag the whole corpus for the fleet being small
	}
	var findings []Finding
	for _, f := range corpus {
		if f.Host != fragment.AxisAny && f.Profile != fragment.AxisAny && f.Harness != fragment.AxisAny {
			continue
		}
		var hits []string
		for _, t := range targets {
			if f.Selects(t.Selector) {
				hits = append(hits, t.Name)
			}
		}
		if len(hits) == 1 {
			findings = append(findings, Finding{
				Check: "narrow-scope-candidate", Severity: SeverityInfo, FragmentID: f.ID,
				Detail: fmt.Sprintf("tagged `any` but only target %s selects it: narrowing the tag makes the scope claim match reality", hits[0]),
			})
		}
	}
	return findings
}

// bloat compiles each target and warns when a file crosses bloatWarnRatio of the
// budget. A compile failure is itself a warn finding — audit must not die on the
// corpus it exists to diagnose.
func bloat(corpus []fragment.Fragment, targets []compile.Target, budget int) []Finding {
	threshold := int(float64(budget) * bloatWarnRatio)
	var findings []Finding
	for _, t := range targets {
		res, err := compile.Compile(corpus, t)
		if err != nil {
			findings = append(findings, Finding{
				Check: "compile-failure", Severity: SeverityWarn, FragmentID: t.Name,
				Detail: err.Error(),
			})
			continue
		}
		var paths []string
		for p := range res.Files {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			if n := len(res.Files[p]); n > threshold {
				findings = append(findings, Finding{
					Check: "bloat", Severity: SeverityWarn, FragmentID: t.Name + "/" + p,
					Detail: fmt.Sprintf("%d bytes, over %d%% of the %d budget: these files inject every session — every byte is a per-session tax", n, int(bloatWarnRatio*100), budget),
				})
			}
		}
	}
	return findings
}

func truncate(s string) string {
	const max = 60
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
