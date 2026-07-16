package compile

import (
	"github.com/cyperx84/soul-forge/internal/fragment"
)

// A RenderMap answers one question per target: given a selected fragment, which file
// does it land in? The map — not the fragment — encodes a harness's file layout.
//
// This split is what lets one fragment reach two harnesses without hand-syncing. "Disk
// chronically tight" is a single fragment; it lands in TOOLS.md on OpenClaw and in a
// section of CLAUDE.md on Claude Code. Rendered duplication is correct. Hand-owned
// duplication is the bug.
type RenderMap struct {
	// Name identifies the layout ("openclaw", "claude-global").
	Name string

	// Route returns the output path a fragment renders into, and whether it renders
	// at all. Returning false drops the fragment from this target's output — used
	// where a harness has no home for a kind (Claude Code takes no voice: it is a
	// tool, not a character).
	Route func(f fragment.Fragment, s fragment.Selector) (path string, ok bool)

	// Order is the output file order for deterministic rendering.
	Order []string

	// Headers are per-file titles.
	Headers map[string]string

	// Skeleton lists paths compile writes as an empty scaffold and never overwrites.
	// MEMORY.md is the case: it holds lifecycle:instance fragments, written at
	// runtime. Compile owns its existence, never its contents.
	Skeleton []string
}

// OpenClawRenderMap is the seven-file workspace layout.
//
// Doc basis: concepts/agent-workspace.md (file roles), concepts/soul.md ("Keep
// AGENTS.md for operating rules. Keep SOUL.md for voice, stance, and style").
//
// Route order is significant and encodes precedence, not preference:
//   - identity first, so an agent's role card never gets swallowed by a rules file
//   - user facts next: they are about the human, and USER.md is their only home
//   - machine facts next: a fact tagged to this host belongs in TOOLS.md whatever
//     its kind — TOOLS is the per-machine layer, and docs call it "local tool
//     conventions… only guidance", conventions included, not facts-only
//   - voice, then rules last as the catch-all
func OpenClawRenderMap() RenderMap {
	return RenderMap{
		Name: "openclaw",
		Route: func(f fragment.Fragment, s fragment.Selector) (string, bool) {
			switch {
			case f.Kind == fragment.KindIdentity:
				return "IDENTITY.md", true
			case f.Profile == fragment.ProfileUser:
				return "USER.md", true
			case f.Host != fragment.AxisAny:
				// Host-specific: this machine's layer, regardless of kind.
				return "TOOLS.md", true
			case f.Kind == fragment.KindVoice:
				return "SOUL.md", true
			default:
				return "AGENTS.md", true
			}
		},
		Order: []string{"AGENTS.md", "SOUL.md", "IDENTITY.md", "USER.md", "TOOLS.md", "MEMORY.md", "HEARTBEAT.md"},
		Headers: map[string]string{
			"AGENTS.md":    "AGENTS.md",
			"SOUL.md":      "SOUL.md — voice and stance only",
			"IDENTITY.md":  "IDENTITY.md",
			"USER.md":      "USER.md",
			"TOOLS.md":     "TOOLS.md — this machine",
			"MEMORY.md":    "MEMORY.md — curated, durable",
			"HEARTBEAT.md": "HEARTBEAT.md",
		},
		Skeleton: []string{"MEMORY.md", "HEARTBEAT.md"},
	}
}

// ClaudeGlobalRenderMap emits one sectioned file: ~/.claude/CLAUDE.md.
//
// No kind:voice, and it is a design position, not an omission: a coding harness is
// a tool, and a tool gets no persona. A voice fragment reaching this target is
// dropped at the map, not filtered upstream — the corpus stays harness-neutral.
func ClaudeGlobalRenderMap() RenderMap {
	const out = "CLAUDE.md"
	return RenderMap{
		Name: "claude-global",
		Route: func(f fragment.Fragment, s fragment.Selector) (string, bool) {
			if f.Kind == fragment.KindVoice {
				return "", false
			}
			return out, true
		},
		Order:   []string{out},
		Headers: map[string]string{out: "CLAUDE.md"},
	}
}

// HermesRenderMap emits one file: SOUL.md — doctrine and voice together.
//
// This is not the OpenClaw split relaxed by accident. Hermes has no home-level
// AGENTS.md and cannot have one (its subdirectory hints scan only the working
// tree, explicitly to prevent cross-agent context contamination), so SOUL.md is
// the only authored slot the harness reads at home scope. "SOUL.md is voice only"
// is an OpenClaw rule — a filename only means what the harness around it says it
// means. USER.md is deliberately absent, skeleton included: Hermes' memory tool
// writes it at runtime, and a compile target that named it would clobber real
// memory (lifecycle:instance, invariant 3).
func HermesRenderMap() RenderMap {
	const out = "SOUL.md"
	return RenderMap{
		Name: "hermes",
		Route: func(f fragment.Fragment, s fragment.Selector) (string, bool) {
			return out, true
		},
		Order:   []string{out},
		Headers: map[string]string{out: "SOUL.md"},
	}
}

// CodexRenderMap emits one file: AGENTS.md. Same tool-not-character position as
// the claude map: voice is dropped at the map.
func CodexRenderMap() RenderMap {
	const out = "AGENTS.md"
	return RenderMap{
		Name: "codex",
		Route: func(f fragment.Fragment, s fragment.Selector) (string, bool) {
			if f.Kind == fragment.KindVoice {
				return "", false
			}
			return out, true
		},
		Order:   []string{out},
		Headers: map[string]string{out: "AGENTS.md"},
	}
}
