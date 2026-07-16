# soul-forge

Compile AI-agent instruction files from a tagged fragment corpus — write a rule once, render it per agent, per machine, per harness.

Agent instruction files (`AGENTS.md`, `SOUL.md`, `CLAUDE.md`, `TOOLS.md`, …) rot the same way everywhere: the same rule gets hand-copied between files, the copies drift, and nobody notices until an agent misbehaves. soul-forge treats the *line*, not the file, as the unit of ownership. Every instruction is a **fragment** — one sentence, tagged with where it applies — and the files agents actually read are **compiled render targets**.

```
fragment:  "Prefer trash over rm."
tags:      host=any  profile=any  harness=any  kind=rule

           ↓ compile

openclaw workspace  →  AGENTS.md      (rule, for every agent)
claude code         →  ~/.claude/CLAUDE.md
hermes              →  ~/.hermes/SOUL.md
```

Change the fragment, recompile, every harness updates. No hand-sync, no drift.

soul-forge is a **deterministic CLI** and never calls an LLM. The two interview steps emit *briefs* — prompts you hand to whatever voice model you talk to — and parse the answers back. The reasoning layer is swappable; the compiler is not.

## Install

```bash
brew install cyperx84/tap/soul-forge     # Homebrew (macOS / Linux)
npx soul-forge --help                     # npm, no install
go install github.com/cyperx84/soul-forge@latest
```

## The model

Every fragment carries five tags:

| tag | values | question it answers |
|-----|--------|---------------------|
| `host` | machine id / `any` | which machine is this true on? |
| `profile` | agent id / `user` / `any` | which agent does this belong to? |
| `harness` | `openclaw` `claude` `hermes` `codex` / `any` | which runtime is this specific to? |
| `lifecycle` | `authored` / `instance` | doctrine, or runtime-written memory? |
| `kind` | `rule` `fact` `voice` `identity` | what type of sentence is it? |

A compile target is a concrete point in that space — `(host=m1, profile=builder, harness=openclaw)` — plus a render map that decides which file each kind lands in. Selection is asymmetric: `any` flows down to a concrete target, but a `profile=alice` fragment can never reach agent bob. Role bleed is structurally impossible, not policed.

Five invariants fail the build rather than warn, each encoding a real failure mode: role bleed, sub-agent self-sufficiency, instance immunity (compiled output never touches runtime memory), runtime non-duplication, and per-file byte budgets.

## Two ways in

**Migrate existing files:**

```bash
# 1. See what you have — proposed tags + ranked duplicate pairs
soul-forge ingest ~/.openclaw/workspace ~/.claude/CLAUDE.md --host my-box --agents alice,bob

# 2. Batch the decisions the tool refused to guess (one question per file section)
soul-forge review <same paths + flags> --out questions.json
#    ...or emit a voice-interview brief instead of a form:
soul-forge review <same paths + flags> --interview --out brief.md

# 3. Answer, then turn answers into a corpus
soul-forge review <same paths + flags> --answers questions.json --out corpus.json

# 4. Collapse cross-file duplicates into single wide-scoped fragments
soul-forge merge ...
```

**Start from nothing** — a live voice interview authors the corpus:

```bash
# Emit an onboarding brief; optionally scan existing context to warm it up
soul-forge onboard ~/dotfiles ~/notes --host my-box --out brief.md

# Paste brief.md into a voice-mode LLM, talk for ~25 minutes,
# paste its fragment log back:
soul-forge onboard --answers log.txt --out corpus.json
```

## Compile, check, write

```bash
# Drift detector — cron/CI friendly: exit 0 clean, 1 drift, 2 check failed
soul-forge diff --corpus corpus.json --root ~/.openclaw/workspace \
  --host my-box --profile alice --harness openclaw

# Write (dry-run by default; --commit writes; overwrites need --force and
# print byte deltas so a gutted file is unmissable)
soul-forge apply --corpus corpus.json --root ~/.openclaw/workspace \
  --host my-box --profile alice --harness openclaw --commit
```

Targets can also live in a file instead of flags:

```bash
# targets.json: [{"name":"hub","host":"my-box","profile":"alice","harness":"openclaw"}]
soul-forge apply --corpus corpus.json --root ... --targets targets.json --target hub
```

There are no built-in targets on purpose: a default target is somebody's setup baked into the tool, and writing it lands in a real workspace.

## Audit

```bash
# Advisory lint — exit 0 clean, 1 warnings (--strict: any finding), 2 error
soul-forge audit corpus.json --targets targets.json --provenance
```

Reports what shouldn't stop a build but will rot one: duplicate fragments (same
text, two ids — the hand-sync surviving inside the corpus), current-project state
pinned where it goes stale, rules that say "as appropriate", harness-behavior
claims with no `source:` citation, `any` tags that only ever reach one target,
and files nearing their byte budget.

## Provision a new machine or agent

Profiles are corpus files with inheritance: `{"name": ..., "extends": <parent>, "fragments": [...]}`.
Every command that takes `--corpus` accepts either format.

```bash
# prime.json holds your universal fragments. Derive a machine, then an agent:
soul-forge clone prime.json --as m1 \
  --set  "disk-space=Disk is roomy on this box." \
  --retag "disk-space=host:m1"
soul-forge clone m1.json --as builder

# builder.json compiles with every prime rule, m1's overrides, zero copied text
soul-forge apply --corpus builder.json --root ... --host m1 --profile builder --harness openclaw
```

A clone starts empty — inheritance carries the parent's fragments, so a fix in
prime reaches every downstream box on its next compile. Overrides are recorded
and reported by `audit`, never silent. The `extends` path is relative, so the
profile tree survives being copied onto the machine it provisions.

## Safety properties

- `apply` is dry-run by default; `--commit` writes, `--force` gates overwrites, backups are on unless `--no-backup`.
- Runtime-owned memory (`lifecycle:instance` paths, e.g. `MEMORY.md`) is never overwritten and never counts as drift.
- An unreadable file is an error, never "missing" — apply must not overwrite content it never read.
- Every overwrite prints its byte delta (`6625 -> 176 bytes, -97%`): a corpus gap that guts a rulebook looks exactly like a routine re-apply to a boolean gate, and nothing else.
- An empty interview log is an error, not an empty corpus.

## v1 → v2

v2 is a hard break. v1 generated persona files (tensions, calibration, character rubrics); v2 deletes that model entirely — agents are tools with voices, not characters — and replaces generation with compilation. Every v1 command is gone. Migration: run `ingest` + `review` over your existing generated files; they become a corpus like any other hand-written set.

## License

MIT
