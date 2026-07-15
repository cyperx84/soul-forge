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
// No kind:voice. Chris's call, and it is a design position, not an omission: Claude
// Code is a tool, and a tool gets no persona. A voice fragment reaching this target
// is dropped at the map, not filtered upstream — the corpus stays harness-neutral.
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
