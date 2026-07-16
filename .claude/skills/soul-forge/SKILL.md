---
name: soul-forge
description: Compile AI-agent instruction files (AGENTS.md, SOUL.md, CLAUDE.md, TOOLS.md) from a tagged fragment corpus using the soul-forge CLI. Use when the user wants to migrate existing agent files into a corpus, run a soul-forge tagging or onboarding interview, deduplicate rules across harnesses, detect drift between compiled and live files, or apply compiled output. soul-forge never calls an LLM itself — YOU drive the reasoning steps.
---

# soul-forge: fragment compiler for agent instruction files

soul-forge stores every agent instruction as a *fragment* — one sentence tagged
with `host` / `profile` / `harness` / `lifecycle` / `kind` — and compiles the
files agents read (AGENTS.md, SOUL.md, CLAUDE.md, …) as render targets. The CLI
is deterministic and never calls an LLM; you are the reasoning layer.

## Migrating existing files

```
soul-forge ingest <paths> --host <machine-id> --agents <ids>
    # proposed tags per line + ranked duplicate pairs. Read pairs top-down;
    # stop when they stop being real. Scores are corpus-relative — never
    # compare across runs, never treat as absolute similarity.

soul-forge review <paths> <same flags> --out questions.json
    # one question per (file, section, axis-set). Fill answer.<axis> per
    # question or set skip:true. Suggestions are proposals, not defaults.

soul-forge review <paths> <same flags> --answers questions.json --out corpus.json
    # emits the confirmed fragment corpus

soul-forge merge ...
    # collapse cross-file duplicates into one wide-scoped fragment. A merge is
    # a WIDENING: it sends the line to harnesses that never had it. Confirm
    # the widening, not just "are these the same".
```

For a voice-mode interview instead of a form, add `--interview` to the questions
run: it emits a brief you hand to a full-duplex voice model. The Q-log it
returns maps to questions.json by index — you translate the log into the JSON
answers, then run `--answers`.

## Onboarding from nothing

```
soul-forge onboard [context-paths] --host <id> --agents <ids> --out brief.md
    # brief for a voice LLM; context paths (dotfiles, notes) pre-fill
    # an Observations block so the interview starts warm

soul-forge onboard --answers log.txt --out corpus.json
    # parses the interviewer's fragment log (F: <text> | host=.. profile=..
    # kind=..) into a validated corpus
```

## Compile, check, write

```
soul-forge diff  --corpus corpus.json --root <dir> --host <id> --profile <agent> --harness <openclaw|claude|hermes|codex>
    # exit 0 clean, 1 drift, 2 check failed — cron/CI friendly

soul-forge apply --corpus corpus.json --root <dir> <same target flags> [--commit] [--force]
    # dry-run by default. --commit writes. Overwrites need --force and print
    # byte deltas — treat a large negative delta as a corpus gap, not noise.
```

Named targets: `--targets targets.json --target <name>` where the file is
`[{"name":..,"host":..,"profile":..,"harness":..}]`. There are no built-in
targets; define the user's.

## Rules for you, the driving model

- Never invent tags. If a scope is genuinely undecidable, leave the question
  unanswered and tell the user — the tool treats a blank as "not decided",
  which is the truth.
- Duplicate pairs are evidence, not proof. Two similar lines may legitimately
  diverge per-harness; only the user confirms a merge.
- Never apply with --force on the user's behalf without showing them the byte
  deltas first.
- MEMORY.md and other runtime-written files are lifecycle:instance — the
  compiler never touches them, and neither should your edits.
