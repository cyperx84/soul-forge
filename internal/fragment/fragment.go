// Package fragment defines soul-forge v2's core data model.
//
// Ownership is a property of a fragment, not of a file. Files are render targets.
// This inverts v1, where a rule's home file was its identity — a model that broke
// twice when built by hand (see the openclaw workspace fleet/agent-files-rewrite.md,
// rounds 1-2): v1 sorted lines by content type, v2-by-hand sorted by host scope, and
// both were single-axis answers to a multi-axis question. A file can only ever be one
// bucket, so every "fix the matrix" round re-fails while the file is the unit.
//
// A fragment is one rule or fact tagged along four orthogonal scope axes plus a
// content kind. A target declares a Selector; compile emits every matching fragment
// into whichever file that target's render map assigns. "Klaw orchestrates the fleet"
// tagged profile:klaw simply isn't selected when compiling builder — the bug that
// broke the hand-built matrix becomes structurally impossible rather than
// hand-policed.
package fragment

import (
	"fmt"
	"strings"
)

// AxisAny is the universal axis value: a fragment tagged with it selects for every
// target on that axis. It is valid for host, profile, and harness.
const AxisAny = "any"

// Profile axis values with defined meaning. Any other value is an agent id
// (profile:klaw, profile:builder) and selects only when compiling that agent.
const (
	// ProfileUser marks a fragment about the human, not about any agent. It selects
	// universally — every agent needs to know who it works for. The render map, not
	// selection, is what sends it to USER.md rather than AGENTS.md.
	ProfileUser = "user"
)

// Harness axis values. A fragment tagged with a specific harness selects only when
// compiling a target for that harness.
const (
	HarnessOpenClaw = "openclaw"
	HarnessClaude   = "claude"
	HarnessHermes   = "hermes"
	HarnessCodex    = "codex"
)

// Lifecycle separates what the compiler owns from what the runtime owns.
const (
	// LifecycleAuthored is doctrine: stable, compiled, owned by the fragment corpus.
	LifecycleAuthored = "authored"
	// LifecycleInstance is memory written at runtime. Compile must never emit it and
	// diff must never call it drift — runtime memory is not the compiler's business.
	LifecycleInstance = "instance"
)

// Kind is the content discriminator. Scope alone cannot separate SOUL from AGENTS
// when both are profile:klaw, harness:openclaw — content type was never wrong as a
// signal, it was insufficient on its own.
const (
	KindRule     = "rule"
	KindFact     = "fact"
	KindVoice    = "voice"
	KindIdentity = "identity"
)

// NeededBySubagent marks a fragment a delegated worker cannot operate without.
// OpenClaw sub-agent sessions inject only AGENTS.md and TOOLS.md (concepts/
// system-prompt.md:227), so such a fragment rendering anywhere else is a compile
// error: an actionable rule sitting in SOUL.md never reaches a delegated worker.
const NeededBySubagent = "subagent"

// Fragment is one rule or fact, tagged along four orthogonal scope axes plus a kind.
// Every axis is required — an untagged fragment has no defined home, which is the
// v1 bug in a new costume.
type Fragment struct {
	ID   string `json:"id"`
	Text string `json:"text"`

	// Scope axes. Each is independent; none implies another.
	Host      string `json:"host"`      // AxisAny or a machine id ("m4-mini")
	Profile   string `json:"profile"`   // AxisAny, ProfileUser, or an agent id ("klaw")
	Harness   string `json:"harness"`   // AxisAny or a Harness* value
	Lifecycle string `json:"lifecycle"` // LifecycleAuthored or LifecycleInstance

	Kind string `json:"kind"` // Kind* value

	// Source is provenance: the doc line justifying this fragment, as "path:line".
	// Optional, but audit --provenance reports fragments asserting harness behavior
	// without one. Its absence produced a real review error: a citation resolving a
	// different question laundered a wrong call as confirmed.
	Source string `json:"source,omitempty"`

	// NeededBy names contexts that must receive this fragment; see NeededBySubagent.
	NeededBy []string `json:"needed_by,omitempty"`
}

// Selector is what a compile target declares: the concrete point in scope space it
// compiles for. There is no Lifecycle field — instance fragments never compile, so a
// target cannot ask for them.
type Selector struct {
	Host    string // concrete machine id
	Profile string // concrete agent id
	Harness string // concrete Harness* value
}

// Selects reports whether f belongs in a target compiled for s.
//
// An axis matches when the fragment is AxisAny (universal) or names s's exact value.
// Fragments never select "upward": a host:m4-mini fragment does not reach m1. That
// asymmetry is the whole point — it is what makes role bleed impossible instead of
// merely discouraged.
func (f Fragment) Selects(s Selector) bool {
	// Instance immunity: runtime memory is never compiled, whatever its other tags.
	if f.Lifecycle == LifecycleInstance {
		return false
	}
	return matchAxis(f.Host, s.Host) &&
		matchProfile(f.Profile, s.Profile) &&
		matchAxis(f.Harness, s.Harness)
}

func matchAxis(fragValue, targetValue string) bool {
	return fragValue == AxisAny || fragValue == targetValue
}

// matchProfile is matchAxis plus the ProfileUser exception: facts about the human
// select for every agent, because every agent works for the same human.
func matchProfile(fragValue, targetValue string) bool {
	if fragValue == ProfileUser {
		return true
	}
	return matchAxis(fragValue, targetValue)
}

var (
	validHarness   = []string{AxisAny, HarnessOpenClaw, HarnessClaude, HarnessHermes, HarnessCodex}
	validLifecycle = []string{LifecycleAuthored, LifecycleInstance}
	validKind      = []string{KindRule, KindFact, KindVoice, KindIdentity}
)

// Validate rejects a fragment that cannot be placed. Host and profile are open
// vocabularies (machine ids, agent ids), so they are checked for presence only;
// harness, lifecycle, and kind are closed sets and are checked against them.
func (f Fragment) Validate() error {
	if strings.TrimSpace(f.ID) == "" {
		return fmt.Errorf("fragment: id is required")
	}
	if strings.TrimSpace(f.Text) == "" {
		return fmt.Errorf("fragment %q: text is required", f.ID)
	}
	if strings.TrimSpace(f.Host) == "" {
		return fmt.Errorf("fragment %q: host is required (use %q for every machine)", f.ID, AxisAny)
	}
	if strings.TrimSpace(f.Profile) == "" {
		return fmt.Errorf("fragment %q: profile is required (use %q for every agent)", f.ID, AxisAny)
	}
	if err := oneOf(f.Harness, validHarness); err != nil {
		return fmt.Errorf("fragment %q: harness: %w", f.ID, err)
	}
	if err := oneOf(f.Lifecycle, validLifecycle); err != nil {
		return fmt.Errorf("fragment %q: lifecycle: %w", f.ID, err)
	}
	if err := oneOf(f.Kind, validKind); err != nil {
		return fmt.Errorf("fragment %q: kind: %w", f.ID, err)
	}
	return nil
}

func oneOf(value string, allowed []string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("is required, want one of %s", strings.Join(allowed, ", "))
	}
	return fmt.Errorf("%q is not one of %s", value, strings.Join(allowed, ", "))
}

// Select returns every fragment in corpus that belongs in a target compiled for s,
// preserving corpus order so compile output is deterministic.
func Select(corpus []Fragment, s Selector) []Fragment {
	var out []Fragment
	for _, f := range corpus {
		if f.Selects(s) {
			out = append(out, f)
		}
	}
	return out
}
