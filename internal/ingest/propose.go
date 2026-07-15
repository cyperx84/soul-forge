package ingest

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

// Tag is a proposed value for one scope axis, with the evidence behind it.
//
// Certain is the load-bearing field. It does not mean "probably right" — it means a
// deterministic signal *decided* this axis (the file is named SOUL.md, so the kind is
// voice). When no signal fires, the axis carries a fallback value and Certain is
// false, and Confirm refuses to build a Fragment until a human or harness supplies
// it. A confidence score would invite exactly the behaviour the spec forbids:
// shipping the guess because the number looked high enough.
type Tag struct {
	Value  string
	Reason string
	// Certain reports whether a signal determined this value. False means the value
	// is a placeholder awaiting judgment, not a weak opinion.
	Certain bool
}

// Proposal is one candidate line with a proposed tag per axis. It is deliberately not
// a Fragment and cannot be selected, rendered, or compiled — the compiler's API takes
// []fragment.Fragment, and the only way to obtain one from here is Confirm.
type Proposal struct {
	Candidate Candidate

	Host      Tag
	Profile   Tag
	Harness   Tag
	Lifecycle Tag
	Kind      Tag

	// Flags are non-blocking observations worth a reviewer's attention: a line that
	// names hardware while proposed host:any, a line that restates runtime-injected
	// contract, a suspected secret.
	Flags []string
}

// Unresolved lists the axes no signal decided. A reviewer confirming a proposal only
// has to answer these — the whole point of proposing is to shrink the judgment step
// to what actually needs judgment.
func (p Proposal) Unresolved() []string {
	var out []string
	for _, a := range []struct {
		name string
		tag  Tag
	}{
		{"host", p.Host}, {"profile", p.Profile}, {"harness", p.Harness},
		{"lifecycle", p.Lifecycle}, {"kind", p.Kind},
	} {
		if !a.tag.Certain {
			out = append(out, a.name)
		}
	}
	return out
}

// Confirm turns a reviewed Proposal into a Fragment.
//
// overrides supplies a value per axis name ("host", "profile", "harness",
// "lifecycle", "kind"); anything omitted keeps the proposed value. Every unresolved
// axis must be supplied, or Confirm fails: a proposal with an undecided axis is a
// guess, and a compiled guess is the bug class this model exists to make impossible.
// Passing an override for an axis that was already certain is allowed — a signal can
// be wrong, and a reviewer overruling it is the system working.
func (p Proposal) Confirm(id string, overrides map[string]string) (fragment.Fragment, error) {
	pick := func(name string, t Tag) (string, error) {
		if v, ok := overrides[name]; ok {
			return v, nil
		}
		if !t.Certain {
			return "", fmt.Errorf("%s: unresolved (%s) — must be confirmed, not defaulted", name, t.Reason)
		}
		return t.Value, nil
	}

	f := fragment.Fragment{
		ID:     id,
		Text:   p.Candidate.Text,
		Source: p.Candidate.Origin(),
	}
	var err error
	if f.Host, err = pick("host", p.Host); err != nil {
		return fragment.Fragment{}, err
	}
	if f.Profile, err = pick("profile", p.Profile); err != nil {
		return fragment.Fragment{}, err
	}
	if f.Harness, err = pick("harness", p.Harness); err != nil {
		return fragment.Fragment{}, err
	}
	if f.Lifecycle, err = pick("lifecycle", p.Lifecycle); err != nil {
		return fragment.Fragment{}, err
	}
	if f.Kind, err = pick("kind", p.Kind); err != nil {
		return fragment.Fragment{}, err
	}
	if err := f.Validate(); err != nil {
		return fragment.Fragment{}, err
	}
	return f, nil
}

// Options carries what ingest cannot discover from the files themselves.
//
// Both fields are facts about the fleet, not about the text. Deriving an agent roster
// by scanning for capitalised words would be exactly the silent guess this package
// refuses to make, so the caller states them.
type Options struct {
	// Host is the machine id these files were read from ("m4-mini").
	Host string
	// Agents are known agent ids ("klaw", "builder"). A line naming one is proposed
	// as scoped to it.
	Agents []string
}

// Propose assigns a tag per axis from deterministic signals.
//
// Signals are ranked by how much they actually know. A filename is strong evidence —
// the harness's own docs assign SOUL.md its role, so a line in it is voice. Wording is
// weak evidence and only ever raises a Flag; a line saying "Klaw" might be *about*
// Klaw or might be addressed *to* Klaw, and no regex settles that.
func Propose(c Candidate, opts Options) Proposal {
	p := Proposal{Candidate: c}
	base := fileOf(c.Path)

	p.Harness = proposeHarness(c.Path)
	p.Lifecycle = proposeLifecycle(base)
	p.Kind = proposeKind(base, c.Section)
	p.Profile = proposeProfile(base, c.Text, opts)
	p.Host = proposeHost(base, c.Text, opts)
	p.Flags = flag(c, p)
	return p
}

func fileOf(path string) string {
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		return path[i+1:]
	}
	return path
}

// proposeHarness reads the path. Each harness owns a directory, so the location of a
// file is that harness's own statement about which harness it is for.
func proposeHarness(path string) Tag {
	switch {
	case strings.Contains(path, ".openclaw/"):
		return Tag{fragment.HarnessOpenClaw, "path under .openclaw/", true}
	case strings.Contains(path, ".claude/"), fileOf(path) == "CLAUDE.md":
		return Tag{fragment.HarnessClaude, "Claude Code memory file", true}
	case strings.Contains(path, ".hermes/"):
		return Tag{fragment.HarnessHermes, "path under .hermes/", true}
	case strings.Contains(path, ".codex/"):
		return Tag{fragment.HarnessCodex, "path under .codex/", true}
	}
	return Tag{fragment.AxisAny, "path names no harness", false}
}

// proposeLifecycle is decided by filename alone, and only MEMORY.md decides it.
//
// Doc basis: concepts/agent-workspace.md:91 defines MEMORY.md as "durable facts,
// preferences, decisions, and short summaries" — runtime-accreted, so lifecycle:
// instance. Everything else in a workspace is authored doctrine.
func proposeLifecycle(base string) Tag {
	if base == "MEMORY.md" {
		return Tag{fragment.LifecycleInstance, "MEMORY.md is runtime-written (agent-workspace.md:91)", true}
	}
	return Tag{fragment.LifecycleAuthored, "not a runtime-written file", true}
}

// proposeKind reads the filename, because each harness's docs assign its files a role.
//
// Two files resolve to nothing on purpose. TOOLS.md is documented as "local tool
// conventions… only guidance" (agent-workspace.md:75) — conventions *and* facts, so
// the file cannot decide the kind. CLAUDE.md is one sectioned file holding everything
// Claude Code needs, so it decides even less. Those are the files whose lines need a
// human, and saying so is the honest output.
func proposeKind(base, section string) Tag {
	switch base {
	case "SOUL.md":
		return Tag{fragment.KindVoice, "SOUL.md is voice and stance (concepts/soul.md)", true}
	case "IDENTITY.md":
		return Tag{fragment.KindIdentity, "IDENTITY.md is the agent's role card", true}
	case "AGENTS.md":
		return Tag{fragment.KindRule, "AGENTS.md is operating rules (concepts/soul.md)", true}
	case "USER.md":
		return Tag{fragment.KindFact, "USER.md states facts about the human", true}
	case "MEMORY.md":
		return Tag{fragment.KindFact, "MEMORY.md holds durable facts (agent-workspace.md:91)", true}
	}
	return Tag{fragment.KindRule, base + " mixes kinds; wording alone cannot decide", false}
}

// proposeProfile decides only where the file itself says who a line is about.
//
// USER.md is documented as being about the human, so its lines are profile:user. Every
// other file is ambiguous: AGENTS.md holds both fleet-wide doctrine (profile:any) and
// this agent's own rules (profile:klaw), and that ambiguity is precisely what broke
// the hand-built matrix v2. A named agent raises a Flag rather than deciding the tag.
func proposeProfile(base, text string, opts Options) Tag {
	if base == "USER.md" {
		return Tag{fragment.ProfileUser, "USER.md is about the human", true}
	}
	if base == "IDENTITY.md" {
		return Tag{fragment.AxisAny, "IDENTITY.md names its own agent; which one is the caller's to state", false}
	}
	if named := namedAgents(text, opts.Agents); len(named) > 0 {
		return Tag{
			fragment.AxisAny,
			"names " + strings.Join(named, ", ") + " — about that agent, or addressed to it?",
			false,
		}
	}
	return Tag{fragment.AxisAny, "no agent named; assumed fleet-wide", false}
}

// proposeHost decides from the file's documented role, and refuses in the two places
// that role does not settle it.
//
// TOOLS.md is the per-machine layer, which tempts a host:<this machine> default — and
// that default would be wrong for the lines in it that are harness rules wearing a
// machine's clothes ("never use exec/curl for provider messaging" is true on every
// box). Under matrix v3 those route to AGENTS.md, not TOOLS.md. Getting this wrong
// pins a fleet-wide rule to one machine, and it silently vanishes from every other
// box — the exact failure the role-bleed invariant exists to catch, one axis over.
//
// Every *other* workspace file is documented as not being the per-machine layer, so
// its lines are host:any on the same evidence that makes SOUL.md voice: the harness's
// own docs assign the file its role. Refusing to decide there is not caution, it is
// noise — it hands a reviewer 144 questions with one answer and gets the migration
// abandoned. A line naming hardware still needs a human whatever file it is in.
func proposeHost(base, text string, opts Options) Tag {
	if hardware(text) {
		return Tag{opts.Host, "names hardware or capacity specific to one box", false}
	}
	if base == "TOOLS.md" || base == "CLAUDE.md" {
		return Tag{fragment.AxisAny, base + " mixes this machine's facts with fleet-wide rules", false}
	}
	return Tag{fragment.AxisAny, base + " is not the per-machine layer (agent-workspace.md:75)", true}
}

var (
	// hardwareRe matches capacity and Apple-silicon model markers: the wording that
	// pins a line to one box.
	hardwareRe = regexp.MustCompile(`(?i)\b(\d+\s?(GB|TB|MB)|M[1-9]\s?(Pro|Max|Ultra|Mini)?|Mac\s?Mini|MacBook|Apple Silicon)\b`)
	// modelRe matches provider/model strings. The spec forbids hardcoded model names
	// outright: routing flips often, so a model name in an instruction file is a
	// fact with an expiry date.
	modelRe = regexp.MustCompile(`(?i)\b(claude|gpt|gemini|llama|sonnet|opus|haiku|kimi|glm)[-\w.]*-?\d`)
	// secretRe matches key-shaped strings. Promoted from v1's audit heuristic.
	secretRe = regexp.MustCompile(`\b(sk-[A-Za-z0-9]{16,}|ghp_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16})\b`)
	// runtimeRe matches contract each harness injects at run time. Duplicating it is
	// a compile error (invariant 4); groups.md:456 says so verbatim for NO_REPLY.
	runtimeRe = regexp.MustCompile(`(?i)\b(NO_REPLY|HEARTBEAT_OK|ANNOUNCE_SKIP)\b`)
)

func hardware(s string) bool { return hardwareRe.MatchString(s) }

func namedAgents(text string, agents []string) []string {
	var out []string
	lower := strings.ToLower(text)
	for _, a := range agents {
		if strings.Contains(lower, strings.ToLower(a)) {
			out = append(out, a)
		}
	}
	sort.Strings(out)
	return out
}

// flag raises non-blocking observations. These are the lines a reviewer should read
// first: each one is a bug that already shipped once.
func flag(c Candidate, p Proposal) []string {
	var out []string
	if secretRe.MatchString(c.Text) {
		out = append(out, "possible secret — must never enter a profile or an output")
	}
	if modelRe.MatchString(c.Text) {
		out = append(out, "names a model — routing is config, not doctrine; look it up at runtime")
	}
	if runtimeRe.MatchString(c.Text) {
		// Deliberately "names", not "restates". A regex sees the token and cannot
		// tell a restatement from a prohibition — the line "never restate NO_REPLY;
		// the runtime injects it" trips this, and calling that a violation would be
		// the flag asserting more than it knows. That is the exact failure this
		// project keeps re-teaching, so the wording stops at what was observed.
		out = append(out, "names runtime-injected contract — restating it is a compile error (invariant 4; groups.md:456); check whether this line restates it or forbids restating it")
	}
	if hardware(c.Text) && p.Host.Value == fragment.AxisAny {
		out = append(out, "names hardware but proposed host:any — would pin one box's fact to the fleet")
	}
	return out
}
