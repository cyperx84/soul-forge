# soul-forge v2 — spec

Born 2026-07-13 from the Klaw agent-files rewrite (`#agent-files-rewrite`, project doc: openclaw workspace `fleet/agent-files-rewrite.md`). v2 codifies that workflow: **audit existing files → spec from harness docs → scope-tag every rule → compile per harness → cross-review → detect drift forever**.

Revised 2026-07-15 after two rounds of hand-building the thing this tool is supposed to build. Both rounds failed, identically, and the core data model below is the fix. Read "The fragment model" before anything else — the rest is plumbing.

## Reframe (v1 → v2)

v1 is a persona generator: interview → profile → opinionated SOUL.md personas per role.

v2 is an **agent-file compiler**: one profile source of truth → every guidance file on every machine, for every harness — aligned to the user, backed by each harness's official docs, deduplicated by construction, drift-detected by diff.

Personas are out. Chris's call, verbatim intent: agents are tools with voices, not characters. `kind:voice` fragments carry tone, bluntness, opinions, boundaries — no creature bios, no "you're becoming someone," no example-dialogue theater.

### Persona removal is a hard break (decided 2026-07-15)

The persona machinery is not on a branch to abandon — it is **live on main and shipped** via brew/npm: `cmd/rubric.go`, `internal/rubric/`, `internal/soulmd/`, plus `tensions`/`calibration`/`predictiveness` in `internal/schema/profile.schema.json`, `internal/config`, and templates. `rubric`'s own help text says it scores *"how well the agent stays in character"* — the model this spec rejects, running in a published tool.

So v2 is a breaking change to published surface, not a cleanup. Chris's call: **hard break, major version bump, no deprecation shim.** Rationale: the tool is v0.x, the persona surface contradicts the tool's own stated purpose, and carrying a compatibility shim for machinery nobody should use costs more than the break. `generate`'s persona-depth machinery is deleted outright — not demoted to an off-by-default flag.

Version number is a release-time decision, deliberately not pinned here. Note the naming collision for whoever picks it: the ownership matrix is on its **v3**, this spec file is named **V2-SPEC**, and the shipped tool is **v0.1.0**. Three unrelated "v" numbers. The matrix version is a design-history label and has no business becoming the tool's tag.

## The fragment model (core data model)

**Ownership is a property of a fragment, not of a file. Files are render targets.**

This is the finding that killed two hand-built ownership matrices (see `fleet/agent-files-rewrite.md`, rounds 1–2). v1 sorted lines by content type; v2 sorted by host scope. Both were single-axis answers to a multi-axis question, and both broke on the same class of line. Every "fix the matrix" round re-fails while the file is the unit, because a file can only ever be one bucket.

**This spec's first draft made the same mistake one level up.** L1 user / L2 doctrine / L3 harness / L4 voice is an *ordinal stack* — it collapses independent axes into a single dimension of generality. L3 was the tell: "orchestration rules for a hub, env facts for a machine, keybindings for Claude Code" is three unrelated axes crammed into one layer. Replaced.

The unit is a **fragment**: one rule or fact, tagged along four orthogonal axes plus a content kind.

| axis | values |
|---|---|
| `host` | `any` / `<machine-id>` |
| `profile` | `any` / `user` / `<agent-id>` |
| `harness` | `any` / `openclaw` / `claude` / `hermes` / `codex` |
| `lifecycle` | `authored` (compiled, stable) / `instance` (written at runtime, never compiled) |
| `kind` | `rule` / `fact` / `voice` / `identity` |

A target declares a scope selector; compile emits every matching fragment into the file its render map assigns. `kind` is a content-type tag and it is still needed — scope alone can't separate SOUL from AGENTS when both are `profile:klaw, harness:openclaw`. Content type was never *wrong*, it was insufficient alone.

Worked examples, using the lines that actually broke:

- *"Klaw orchestrates the fleet"* → `host:any, profile:klaw, harness:openclaw, kind:identity`. Compiling Builder (`profile:builder`) doesn't select it. The matrix-v2 bug becomes structurally impossible rather than hand-policed.
- *"Disk chronically tight (228GB, often 90%+)"* → `host:m4-mini, profile:any, harness:any, kind:fact`. Renders into M4's TOOLS.md **and** M4's CLAUDE.md — one fragment, two outputs. Rendered duplication is correct; hand-owned duplication is the bug.
- *"trash > rm"* → `host:any, profile:any, harness:any, kind:rule`. Every output, every machine, one source.

Single-owner is enforced by construction: a fragment exists once. Two outputs sharing a string is *expected* when their selectors overlap — that's a render, not a duplicate. `audit` flags duplicate **fragments**, never duplicate strings.

`lifecycle` is the axis that settles the question hand-review couldn't: does MEMORY own lessons? No. A lesson that changes what you do next is a rule (`authored`), so it compiles into AGENTS. MEMORY holds `instance` fragments only — and compile must never write them, diff must never call them drift. Doc-backed: `concepts/agent-workspace.md:91` defines MEMORY.md as "durable facts, preferences, decisions, and short summaries."

### Provenance

A fragment may carry `source: <doc-path:line>` — the doc line that justifies it. This is what made the hand-review defensible, and its absence is what produced the round-2 error: a citation that resolves a *different* question launders a wrong call as confirmed. `audit --provenance` reports fragments asserting harness behavior with no citation. Cheap to store; it makes the next reviewer's job checkable instead of re-derivable.

## Profile store

```
.soul-forge/
  profile.json          # identity + extends pointer
  fragments/*.json      # the corpus: every rule and fact, each scope-tagged
  soul.json             # compile manifest: targets, selectors, output paths, budgets
```

Fragments are flat, not filed by target — filing them by target would reintroduce the file-as-owner bug in the store itself. Selection is by tag query; grouping is the render map's job.

Profiles inherit: `"extends": "<path-or-name>"`. Provisioning maps 1:1 — prime (M1, clean, GPG keys born there) holds every `host:any, profile:any` fragment; each machine's profile adds `host:<id>` fragments; each agent adds `profile:<id>` rule/voice/identity fragments. `clone` derives a child that can add and override but never silently drops a parent fragment.

## Compile targets (encode official harness layouts)

Target specs baked in from harness docs, not guesses:

| target | emits | doc basis |
|---|---|---|
| `openclaw-hub` | AGENTS.md SOUL.md USER.md IDENTITY.md TOOLS.md MEMORY.md(skeleton) HEARTBEAT.md | openclaw docs: concepts/agent-workspace, concepts/soul ("AGENTS.md = operating rules, SOUL.md = voice/stance/style"), standing-orders |
| `openclaw-worker` | same set; selector swaps `profile:<agent>`, inherits every `profile:any` rule | same |
| `claude-global` | ~/.claude/CLAUDE.md — selector `harness:any|claude`, `kind:rule|fact`; **no `kind:voice`** (pure tool, Chris's call) | Claude Code memory docs |
| `hermes` | ~/.hermes/SOUL.md **only** — and it carries `kind:rule` as well as `kind:voice` (see below). Never USER.md. | hermes source: `agent/system_prompt.py:150-162`, `agent/subdirectory_hints.py:169-176`, `tools/memory_tool.py:189,284,309` |
| `codex-global` | ~/.codex/AGENTS.md | codex AGENTS.md convention |
| `generic` | soul.md / AGENTS.md pair | agents.md convention |

Token budgets per target in soul.json (openclaw defaults: 20k chars/file, 60k total; injected every session → every char is per-session tax). `audit` warns at threshold.

### Render maps

A render map answers one question per target: given a selected fragment, which file does it land in? The map — not the fragment — encodes each harness's file layout, doc-derived.

`openclaw`: `kind:rule` → AGENTS.md · `host:<this machine>` → TOOLS.md · `kind:voice` → SOUL.md · `profile:user` → USER.md · `kind:identity` → IDENTITY.md · `lifecycle:instance` → MEMORY.md (skeleton only, never overwritten).

`claude-global`: every selected fragment renders into one sectioned file. `codex-global`: AGENTS.md.

`hermes`: SOUL.md only — and it is the case that proves render maps have to exist.

Hermes has exactly one home-level authored slot. `system_prompt.py:150-162` loads SOUL.md as the stable tier's first part, falling back to a hardcoded `DEFAULT_AGENT_IDENTITY` when it is empty. There is no home AGENTS.md and there cannot be one: `subdirectory_hints.py:169-176` only scans within the working directory tree, and says why — *"This prevents loading AGENTS.md from outside the active workspace (e.g. ~/.codex/AGENTS.md, ~/.claude/CLAUDE.md), which causes cross-agent context contamination and instruction mixup."*

So the hermes map routes **`kind:rule` into SOUL.md alongside `kind:voice`**, and that is not a contradiction of "SOUL.md is voice only". That rule is an *OpenClaw* rule, true because OpenClaw injects AGENTS.md next to SOUL.md. Hermes has no such neighbour. The file name is identical and the correct content is not — which is exactly why ownership cannot live in a filename. Copying OpenClaw's SOUL.md to `~/.hermes/SOUL.md` would deliver voice and zero doctrine: every red line silently absent. It is the sub-agent self-sufficiency bug (invariant 2) in a different costume — content is only correct relative to what gets injected beside it.

**`hermes` must never emit USER.md.** `~/.hermes/memories/USER.md` is agent-written at runtime — `tools/memory_tool.py` reads it at `:189`, resolves its path at `:284`, and writes it via `save_to_disk` at `:309`. It is `lifecycle:instance`, so compiling it would clobber real accreted memory and violate invariant 3. The first draft of this table listed it as a compile output; that was a spec bug, caught by reading the source rather than the file names.

Same fragments, different maps. That a machine fact renders to TOOLS.md on OpenClaw and to a section of CLAUDE.md on Claude Code is the map's job, not the author's.

## Compile-time invariants

These fail the build. They are not lint warnings — each one encodes a bug that already happened.

1. **Role bleed.** Compile `builder` on host `m1`; assert the output contains no `profile:klaw` fragment, no `host:m4-mini` fragment, and no `harness:openclaw` fragment in a `claude` target. This is the regression test for the matrix-v2 break. It ships before any other feature — it's the whole reason the model changed.
2. **Sub-agent self-sufficiency** (openclaw targets). `concepts/system-prompt.md:227`, verbatim: *"Sub-agent sessions only inject `AGENTS.md` and `TOOLS.md` (other bootstrap files are filtered out to keep the sub-agent context small)."* So a fragment tagged `needed_by:subagent` that renders anywhere else is a compile error. An actionable rule sitting in SOUL.md never reaches a delegated worker.
3. **Instance immunity.** Compile never writes a `lifecycle:instance` path; diff never reports one as drift. Runtime memory is not the compiler's business.
4. **Runtime non-duplication.** Each harness injects its own contract at run time (OpenClaw: group-chat, `NO_REPLY`, heartbeat, messaging, model aliases — full prompts only; sub-agents run `minimal` and get none of them, `concepts/system-prompt.md:116-128`). Fragments duplicating it are a compile error for that target. `channels/groups.md:456`, verbatim: *"workspace files should not duplicate `NO_REPLY` mechanics."*
5. **No secrets, no hardcoded model names.** v1's audit heuristics, promoted from warning to compile error.

## Commands

Kept from v1: `init`, `questions`, `interview` (harness-driven via bundled skill — CLI stays LLM-free), `import`, `dotfiles`, `schema`.

New / rebuilt:

- `ingest <paths...>` — reverse-compile existing files into fragments; proposes scope tags per line and flags strings found in >1 file (the duplication map done by hand in the Klaw rewrite). Tagging is the one step needing judgment, so ingest proposes and the harness/human confirms — it must not silently guess a scope. This is the migration path; nobody starts from zero.
- `compile [--target X|--all]` — fragments → outputs, deterministic, byte-stable given same inputs. Enforces the invariants above.
- `diff [--target X]` — compiled vs live files on disk. Exit non-zero on drift. The standing drift detector; cron/CI-able.
- `audit` — v1 lint (vague language, bloat) **plus fragment lint**: duplicate fragments (not duplicate strings — overlapping renders are correct), untagged fragments, `authored` fragments that only ever select into one target (candidates for a narrower scope), fragments asserting harness behavior with no `source:` provenance, project-state pinned in an always-injected render.
- `apply [--target X]` — write outputs with backup-first (never clobber: .bak or trash), git-aware (refuses on dirty target repo unless --force).
- `clone <base> --as <name> [--set k=v]` — derive profile (new machine / new agent) from a base. Prime→derived provisioning primitive.

## Hard principles (unchanged from v1, now load-bearing)

- **Never calls an LLM.** Deterministic CLI; the calling harness does reasoning via the skill. No API keys, no model names anywhere.
- **Secrets never enter profiles or outputs.** Profile schema rejects key-shaped strings (existing v1 audit heuristic promoted to import-time validation).
- **Machine facts carry a `host` tag.** Compile refuses a `host:any` fragment that names paths or hardware unique to one box.
- **Backup before write, always.** apply never destroys hand edits silently; diff first.

## Migration

1. `soul-forge ingest ~/.openclaw/workspace ~/.claude/CLAUDE.md ~/.hermes/SOUL.md ~/.codex/AGENTS.md` → fragment corpus + proposed tags + duplication report. `~/.codex/AGENTS.md` is 0 bytes today — Codex runs blind, and it's the free win: it costs one compile.
2. Confirm the proposed tags. This is the judgment step and the only one that can't be mechanical.
3. `compile --all`, `diff`, `apply`
4. Cron `soul-forge diff` on each machine → drift alerts to hub
5. Retire hand-synced copies. CLAUDE.md's "distilled from my OpenClaw and Hermes agent rulebooks" header dies — that sentence is the drift, admitted in writing.

## Non-goals

- No LLM-in-CLI, ever. No provider SDKs.
- No personas/character sheets. Voice ≠ character.
- No secrets management (SECRETS.manifest.md pattern stays separate).
- Per-project repo files (project CLAUDE.md/AGENTS.md) out of scope for v2.0; lint hook candidate for v2.x.

## Build plan

Go codebase stays (cmd/ + internal/, goreleaser, brew+npm+go install paths all work — keep). This is a rewrite of internals around the fragment model, not a language/tooling reset.

Order, and the first item is not negotiable:

1. **Fragment schema + the role-bleed invariant test, red.** ✅ done, then made green by step 4 (branch `v2`). `internal/fragment` (schema, four axes, selector, validation — green) + `internal/compile` (API surface, stub, invariant tests red on `ErrNotImplemented`). Red lives on branch `v2`, not main: CI runs on main pushes and PRs, and a red main is a worse default than a red branch. The test corpus is built from the lines that actually broke — `klaw-orchestrates`, `m4-disk-tight` — not invented fixtures. Asserts both halves: no bleed in, and no over-filtering (an invariant that passes by emitting nothing is a broken compiler, not a passing test).
2. Scope selectors + `extends` inheritance (prime → machine → agent). ✅ `internal/fragment/corpus.go` — Resolve() flattens the chain root-first; overrides take the parent's position (a redefinition, not a re-prioritisation) and are *reported*, never silent; extends-cycle detection; a parent fragment can never be silently dropped (pinned by test).
3. Render maps + target specs, doc-cited. ✅ `internal/compile/render.go` — openclaw (7-file layout) + claude-global (one sectioned file, no `kind:voice`).
4. `compile` for `openclaw-hub` + `claude-global`, green against (1). ✅ All five invariants enforced as build failures. Verified by mutation testing, not just green ticks: disabling `no-secrets` and making profile-matching permissive each make the relevant test fail by name.
5. `diff`.
6. `ingest` — the migration.
7. `audit` fragment rules, `clone`, interview polish.

Ship compile+diff first; ingest second; interview last.

## Reference case

The Klaw workspace rewrite (`fleet/agent-files-rewrite.md`, 2026-07-13 → 07-15) is the acceptance test, and it's a good one because it's a *failure* record, not a success story: two ownership matrices designed by hand, both structurally broken, plus one review error where a doc citation answering a different question got laundered as verification. `ingest` on that workspace + `~/.claude/CLAUDE.md` should reproduce today's files as fragments and independently surface the same duplication map that took two LLM cross-review rounds to find by hand. If it can't, the model is wrong again.
