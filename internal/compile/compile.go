// Package compile turns a scope-tagged fragment corpus into a harness's guidance
// files.
//
// The compiler's job is narrow and its refusals are the valuable part. Selection
// decides what a target may see (internal/fragment); the render map decides where it
// lands (render.go); the invariants below decide when the whole build stops. Each
// invariant encodes a bug that already happened during the hand-built rewrite this
// tool exists to replace — they are compile errors, not lint warnings, because every
// one of them was found by a human reading carefully, and that is exactly the job a
// human should not have.
package compile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

// Target is one compile destination: a concrete point in scope space plus the
// harness layout to render into.
type Target struct {
	// Name identifies the target ("openclaw-hub", "claude-global").
	Name string

	// Selector is the scope point this target compiles for.
	Selector fragment.Selector

	// Render is the harness layout. Zero value means "infer from Selector.Harness",
	// which keeps the invariant tests honest: they declare scope, not plumbing.
	Render *RenderMap

	// MaxBytesPerFile, when non-zero, fails the build for any output exceeding it.
	// Workspace files inject every session above the prompt-cache boundary, so every
	// byte is a per-session tax. OpenClaw's documented budget is 20k/file.
	MaxBytesPerFile int
}

// Result is a compiled target.
type Result struct {
	// Files maps output path to rendered content.
	Files map[string]string

	// Selected is every fragment that survived selection, in corpus order. Carried
	// so invariants and audit assert over fragments rather than parsing rendered
	// strings back into meaning.
	Selected []fragment.Fragment

	// Dropped names fragments that selected for this target's scope but had no home
	// in its render map (a voice fragment on Claude Code). Not an error — a design
	// position — but silent drops are how surprises get built, so they are reported.
	Dropped []fragment.Fragment
}

// InvariantError is a compile-time invariant failure: a bug the model is supposed to
// make impossible, caught escaping anyway.
type InvariantError struct {
	Invariant string
	Fragment  string
	Target    string
	Detail    string
}

func (e *InvariantError) Error() string {
	return fmt.Sprintf("compile %s: invariant %q violated by fragment %q: %s",
		e.Target, e.Invariant, e.Fragment, e.Detail)
}

// Compile renders corpus into t's file set, enforcing the compile-time invariants.
// It is deterministic: same corpus and target in, byte-identical output.
func Compile(corpus []fragment.Fragment, t Target) (Result, error) {
	rm, err := t.renderMap()
	if err != nil {
		return Result{}, err
	}

	if dupes := fragment.DuplicateIDs(corpus); len(dupes) > 0 {
		return Result{}, fmt.Errorf("compile %s: duplicate fragment ids %v: two definitions have no defined precedence", t.Name, dupes)
	}

	selected := fragment.Select(corpus, t.Selector)

	if err := checkInvariants(selected, t, rm); err != nil {
		return Result{}, err
	}

	bodies := map[string][]fragment.Fragment{}
	var dropped []fragment.Fragment
	for _, f := range selected {
		path, ok := rm.Route(f, t.Selector)
		if !ok {
			dropped = append(dropped, f)
			continue
		}
		bodies[path] = append(bodies[path], f)
	}

	files := map[string]string{}
	for _, path := range rm.Order {
		frags, hasContent := bodies[path]
		isSkeleton := contains(rm.Skeleton, path)
		if !hasContent && !isSkeleton {
			continue // no fragments, no scaffold: emit nothing rather than an empty file
		}
		files[path] = render(path, rm, frags, isSkeleton)
	}
	// A route may target a path absent from Order — a map bug, not a corpus bug, but
	// dropping content silently is worse than an unordered file.
	for path, frags := range bodies {
		if _, done := files[path]; !done {
			files[path] = render(path, rm, frags, false)
		}
	}

	if t.MaxBytesPerFile > 0 {
		for _, path := range sortedKeys(files) {
			if n := len(files[path]); n > t.MaxBytesPerFile {
				return Result{}, fmt.Errorf("compile %s: %s is %d bytes, over the %d budget: these files inject every session, so every byte is a per-session tax",
					t.Name, path, n, t.MaxBytesPerFile)
			}
		}
	}

	return Result{Files: files, Selected: selected, Dropped: dropped}, nil
}

func (t Target) renderMap() (RenderMap, error) {
	if t.Render != nil {
		return *t.Render, nil
	}
	switch t.Selector.Harness {
	case fragment.HarnessOpenClaw:
		return OpenClawRenderMap(), nil
	case fragment.HarnessClaude:
		return ClaudeGlobalRenderMap(), nil
	default:
		return RenderMap{}, fmt.Errorf("compile %s: no render map for harness %q; set Target.Render explicitly", t.Name, t.Selector.Harness)
	}
}

// checkInvariants runs the compile-time invariants from V2-SPEC.md over the selected
// set. Every failure names the fragment and the reason, because a build that stops
// without saying which line stopped it just moves the archaeology elsewhere.
func checkInvariants(selected []fragment.Fragment, t Target, rm RenderMap) error {
	for _, f := range selected {
		// 1. Role bleed. Selection should already make this impossible; asserting it
		// here is the belt to that braces. If this ever fires, the model is wrong —
		// which is precisely the news worth having.
		if !f.Selects(t.Selector) {
			return &InvariantError{"role-bleed", f.ID, t.Name,
				fmt.Sprintf("selected despite scope host=%s profile=%s harness=%s", f.Host, f.Profile, f.Harness)}
		}

		// 3. Instance immunity. Runtime memory is not the compiler's business.
		if f.Lifecycle == fragment.LifecycleInstance {
			return &InvariantError{"instance-immunity", f.ID, t.Name,
				"lifecycle:instance fragments are written at runtime and must never be compiled"}
		}

		// 5. No hardcoded model names. Provider routing flips often; a model name in
		// an instruction file is a config fact fossilised into doctrine.
		if model := hardcodedModel(f.Text); model != "" {
			return &InvariantError{"no-hardcoded-models", f.ID, t.Name,
				fmt.Sprintf("names model %q: model names are config, looked up at runtime, never compiled into guidance", model)}
		}

		// 5. No secrets. Ever, anywhere, including examples and fixtures.
		if kind := secretShaped(f.Text); kind != "" {
			return &InvariantError{"no-secrets", f.ID, t.Name,
				fmt.Sprintf("contains a %s: credentials never enter a tracked file", kind)}
		}

		// Machine facts must carry a host tag, or they follow a clone onto a box
		// where they are false.
		if f.Host == fragment.AxisAny {
			if path := machineSpecific(f.Text); path != "" {
				return &InvariantError{"untagged-machine-fact", f.ID, t.Name,
					fmt.Sprintf("host:any but names %q, which is specific to one box: tag it host:<machine-id>", path)}
			}
		}

		// 2. Sub-agent self-sufficiency. OpenClaw injects only AGENTS.md and TOOLS.md
		// into sub-agent sessions (concepts/system-prompt.md:227), so a rule a
		// delegated worker needs is inert anywhere else.
		if t.Selector.Harness == fragment.HarnessOpenClaw && contains(f.NeededBy, fragment.NeededBySubagent) {
			path, ok := rm.Route(f, t.Selector)
			if !ok || (path != "AGENTS.md" && path != "TOOLS.md") {
				return &InvariantError{"subagent-self-sufficiency", f.ID, t.Name,
					fmt.Sprintf("needed_by:subagent but renders to %q; sub-agent sessions inject only AGENTS.md and TOOLS.md (concepts/system-prompt.md:227)", path)}
			}
		}

		// 4. Runtime non-duplication. Each harness injects its own contract at run
		// time; restating it burns tokens every session to say what was already said.
		if mech := runtimeInjected(f.Text, t.Selector.Harness); mech != "" {
			return &InvariantError{"runtime-non-duplication", f.ID, t.Name,
				fmt.Sprintf("duplicates %s, which the harness injects at runtime (channels/groups.md:456: \"workspace files should not duplicate NO_REPLY mechanics\")", mech)}
		}
	}
	return nil
}

// modelNames are the shapes a hardcoded model id takes. Deliberately patterns, not a
// roster: a roster in here would be the exact bug the invariant exists to catch.
var modelNames = []string{
	"claude-opus", "claude-sonnet", "claude-haiku", "claude-fable", "claude-3",
	"gpt-4", "gpt-5", "gpt-6", "o1-preview", "gemini-1", "gemini-2", "gemini-3",
	"llama-3", "deepseek-v", "kimi-k",
}

func hardcodedModel(text string) string {
	lower := strings.ToLower(text)
	for _, m := range modelNames {
		if strings.Contains(lower, m) {
			return m
		}
	}
	return ""
}

// secretPrefixes are unambiguous credential shapes. Kept narrow on purpose: a false
// positive here fails a build, so it must only fire on things that are never anything
// but a secret.
var secretPrefixes = map[string]string{
	"sk-ant-":                    "Anthropic API key",
	"sk-proj-":                   "OpenAI project key",
	"ghp_":                       "GitHub personal access token",
	"gho_":                       "GitHub OAuth token",
	"github_pat_":                "GitHub fine-grained token",
	"xoxb-":                      "Slack bot token",
	"xoxp-":                      "Slack user token",
	"AKIA":                       "AWS access key id",
	"-----BEGIN PRIVATE KEY":     "private key",
	"-----BEGIN RSA PRIVATE":     "RSA private key",
	"-----BEGIN OPENSSH PRIVATE": "OpenSSH private key",
}

func secretShaped(text string) string {
	for prefix, kind := range secretPrefixes {
		if strings.Contains(text, prefix) {
			return kind
		}
	}
	return ""
}

// machinePaths are path shapes that only exist on one box. A fragment naming one
// while tagged host:any is a fact that will be false after the first clone.
var machinePaths = []string{"/Users/", "/Volumes/", "/opt/homebrew/Cellar/"}

func machineSpecific(text string) string {
	for _, p := range machinePaths {
		if i := strings.Index(text, p); i >= 0 {
			end := strings.IndexAny(text[i:], " \t\n\"'`,;)")
			if end < 0 {
				return text[i:]
			}
			return text[i : i+end]
		}
	}
	return ""
}

// runtimeContracts maps a harness to the mechanics it injects itself. Restating these
// in a compiled file is the documented anti-pattern, not a style opinion.
var runtimeContracts = map[string]map[string]string{
	fragment.HarnessOpenClaw: {
		"NO_REPLY":     "NO_REPLY mechanics",
		"HEARTBEAT_OK": "heartbeat reply mechanics",
		"sessionKey":   "messaging plumbing",
		"model alias":  "model alias table",
	},
}

func runtimeInjected(text, harness string) string {
	for token, mech := range runtimeContracts[harness] {
		if strings.Contains(text, token) {
			return mech
		}
	}
	return ""
}

// render turns a file's fragments into its content. Deterministic: no timestamps, no
// map iteration, no host lookups. diff compares compiled output against disk, so any
// nondeterminism here would report drift that isn't there and train the reader to
// ignore the alarm.
func render(path string, rm RenderMap, frags []fragment.Fragment, skeleton bool) string {
	var b strings.Builder

	header := rm.Headers[path]
	if header == "" {
		header = path
	}
	b.WriteString("# " + header + "\n")

	if skeleton && len(frags) == 0 {
		b.WriteString("\n<!-- Written at runtime. soul-forge creates this file and never overwrites it. -->\n")
		return b.String()
	}

	for _, f := range frags {
		b.WriteString("\n- " + f.Text + "\n")
	}
	return b.String()
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
