# soul-forge

Generate sharp, opinionated `SOUL.md`, `USER.md`, `AGENTS.md`, `TOOLS.md`, and `MEMORY.md` files for AI agent fleets — [OpenClaw](https://openclaw.dev), Hermes, or any harness that reads a `soul.md`.

soul-forge is a **deterministic CLI that never calls an LLM provider**. It scaffolds the files, supplies opinionated role-based persona defaults, and audits the result for quality. The *interview* — turning a conversation into a profile — is driven by **your agent harness's own model**, via the bundled [Skill](.claude/skills/soul-forge/SKILL.md). No API keys, no hardcoded models.

Two ways to build a profile:
- **Harness-driven (recommended):** ask Claude Code (or any harness) to run the `soul-forge` skill. Its model interviews you, writes `profile.json`, designs each agent's persona, and runs the pipeline.
- **Manual:** `questions` → fill out → build `profile.json` (see `soul-forge schema`) → `import` → `generate`.

### What makes a good soul file

The agent gets a *persona*, not just a copy of your preferences: a voice, opinions it holds, how it decides under ambiguity, hard boundaries, and example exchanges. Each role (`coding`, `infrastructure`, `orchestrator`, `research`, `general`) ships sharp defaults you can override per agent. `soul-forge audit` flags vague/hedging language, missing persona sections, and bloat.

---

## Install

Pick whichever fits your setup — all give you the same `soul-forge` command.

```bash
# Homebrew (macOS / Linux)
brew install cyperx84/tap/soul-forge

# npm / npx (no install — runs the latest release)
npx soul-forge --help
# ...or install it globally
npm install -g soul-forge

# Go
go install github.com/cyperx84/soul-forge@latest
```

<details>
<summary>Build from source</summary>

```bash
git clone https://github.com/cyperx84/soul-forge
cd soul-forge
go build -o soul-forge .
```
</details>

The Homebrew and npm builds download a prebuilt binary (darwin/linux, amd64/arm64) —
no Go toolchain required. Check your version any time with `soul-forge --version`.

---

## Quick start

```bash
soul-forge init                 # scaffold soul-forge.yaml + .soul-forge/
# easiest path — let your agent harness onboard you:
#   in Claude Code (or any harness): "run the soul-forge skill to onboard me"
soul-forge generate --all       # write the agent files
soul-forge audit --all          # check quality + completeness
```

That's it — your agents now have `SOUL.md`, `IDENTITY.md`, `USER.md`, `AGENTS.md`,
`TOOLS.md`, and `MEMORY.md` under `agents/<name>/`. Read on for how each step works.

---

## Workflow

```
init → [harness interview | manual questions] → import → generate → audit
```

**Harness-driven (recommended):**

```
soul-forge init
# then, in Claude Code or any harness:
"run the soul-forge skill to onboard me"
```

The harness model reads `soul-forge schema` + `soul-forge questions`, interviews you,
writes `profile.json`, fills in each agent's `persona:` in `soul-forge.yaml`, and runs
`import` / `generate` / `audit`. soul-forge itself never contacts a provider.

**Manual:**

1. **`soul-forge init`** — scaffold config + `.soul-forge/`
2. **`soul-forge questions`** — get the onboarding questionnaire (topics)
3. Build a `profile.json` (structure: `soul-forge schema`)
4. **`soul-forge import profile.json`** — load your profile
5. **`soul-forge generate --all`** — generate agent files
6. **`soul-forge audit --all`** — verify quality and completeness

---

## Commands

### `soul-forge init`

Creates `soul-forge.yaml` and `.soul-forge/` directory in the current directory.

```bash
soul-forge init
soul-forge init --no-animation
```

**`soul-forge.yaml` format:**

```yaml
# soul-forge configuration
output_dir: agents
agents:
  - name: assistant
    role: general
    channel: main

  - name: coder
    role: coding            # coding | infrastructure | orchestrator | research | general
    channel: dev
    # Optional persona — overrides/augments the role's sharp defaults.
    # Every field is optional. This is what makes the SOUL.md specific.
    persona:
      vibe: "the codebase's quiet maintainer"   # → IDENTITY.md
      emoji: "🔧"
      backstory: "a pragmatic engineer who's maintained this codebase for years"
      voice: "dry, precise, allergic to filler"
      opinions:
        - "I'd rather delete code than add it."
      boundaries:               # integrity lines, not action rules (those are role-driven → AGENTS.md)
        - "I won't ship a change I can't explain."
      examples:             # few-shot — the highest-leverage field
        - prompt: "Add retries to this call?"
          response: "Done — 3-attempt backoff. Caveat: the endpoint isn't idempotent, so retries could double-charge. Gate it on a request key?"
```

Roles are normalized (`software-engineer` → `coding`, `devops`/`sre` → `infrastructure`, etc.), so natural names work.

---

### `soul-forge` skill (harness-driven onboarding)

The bundled [Skill](.claude/skills/soul-forge/SKILL.md) lets your agent harness run the
whole onboarding with **its own model**. In Claude Code, just ask it to "run the soul-forge
skill." It scans your dotfiles, interviews you adaptively, writes a schema-valid
`profile.json`, designs each agent's persona, and runs `import` / `generate` / `audit`.
soul-forge contacts no provider — the model is whatever harness is running.

---

### `soul-forge schema`

Prints the JSON Schema for `profile.json`, so any harness/LLM knows exactly what to produce.

```bash
soul-forge schema
soul-forge schema > profile.schema.json
```

---

### `soul-forge questions`

Outputs the onboarding questionnaire as markdown or JSON.

```bash
# All 3 parts as markdown (default)
soul-forge questions

# Specific part
soul-forge questions --part 1
soul-forge questions --part 2
soul-forge questions --part 3

# JSON format
soul-forge questions --format json

# Save to file
soul-forge questions > questionnaire.md
```

**Parts:**
- **Part 1 — Who Are You:** Identity, background, goals, communication style
- **Part 2 — How Should I Work:** Workflow, tools, feedback preferences, output style
- **Part 3 — Your Environment:** Hardware, OS, shell, editor, dotfiles

---


### `soul-forge generate`

Generates the OpenClaw/Hermes workspace file set per agent, from your profile and config:

| File | What it is |
|------|------------|
| `SOUL.md` | Who the agent *is* — voice, opinions, stance, integrity boundaries, examples. Loaded first, every session. |
| `IDENTITY.md` | Small identity card: name, vibe, emoji. |
| `USER.md` | Persistent facts about the human (static until changed). |
| `AGENTS.md` | The agent's operating procedure (SOP): session-start + memory routine, numbered operating rules, scope, security. |
| `TOOLS.md` | Local tools/environment cheat-sheet — "where things are." Guidance only; env-var names, never secrets. |
| `MEMORY.md` | Accumulated learnings over time. **Seeded once and never overwritten** on regenerate. |

This matches the [OpenClaw workspace](https://docs.openclaw.ai/concepts/agent-workspace) and [Hermes context-file](https://hermes-agent.nousresearch.com/docs/user-guide/features/personality) conventions: SOUL.md is voice/stance, operational rules live in AGENTS.md (single responsibility, no duplication).

```bash
soul-forge generate --all              # all agents
soul-forge generate --agent coder      # one agent
soul-forge generate --all --dry-run    # preview without writing
```

Files are placed at `<output_dir>/<agent-name>/`. Empty profile sections are omitted
(no "Not specified" filler), and role defaults guarantee a complete SOUL.md even from a
sparse profile.

---

### `soul-forge dotfiles <user/repo>`

Clones a GitHub dotfiles repo and extracts tool/environment information into `.soul-forge/dotfiles.json`.

```bash
soul-forge dotfiles cyperx84/dotfiles
```

Scans for:
- Shell config (`.zshrc`, `.bashrc`, etc.) — detects shell, plugins, prompt, aliases
- Editor config (`init.lua`, `.vimrc`, `settings.json`, etc.)
- Git config (`.gitconfig`) — aliases, user info
- Tool indicators (tmux, alacritty, kitty, starship, mise, homebrew, etc.)

Output is written to `.soul-forge/dotfiles.json` and also printed to stdout (pipeable).

---

### `soul-forge import <profile.json>`

Imports a structured profile JSON into `.soul-forge/profile.json`.

```bash
# Overwrite existing profile
soul-forge import my-profile.json

# Merge with existing profile
soul-forge import partial-update.json --merge
```

**`profile.json` schema:**

```json
{
  "identity": {
    "name": "Ada Lovelace",
    "role": "Staff Engineer",
    "background": "10 years backend, moving into distributed systems",
    "goals": ["Ship v2 before Q3", "Learn Rust properly"],
    "communication_style": "Direct, terse, examples over prose",
    "expertise_areas": ["Go", "Postgres", "API design"],
    "learning_focus": ["Rust", "distributed consensus"],
    "working_hours": "09:00-18:00",
    "timezone": "Europe/London",
    "technical_skill": "expert",
    "articulation": "terse, examples over prose"
  },
  "work_style": {
    "workflow": "Kanban with daily review",
    "decision_style": "bias to action, adjust on feedback",
    "feedback_style": "blunt and direct",
    "collab_style": "async-first, sync when stuck",
    "tools": ["neovim", "tmux", "gh", "lazygit"],
    "languages": ["Go", "SQL", "bash", "some Python"],
    "do_not_do": [
      "don't refactor without being asked",
      "don't add docstrings I didn't ask for",
      "don't use ORM when raw SQL is clearer"
    ],
    "output_preferences": {
      "code": "full blocks, not snippets",
      "explanations": "short prose, then example",
      "lists": "bullet points"
    }
  },
  "environment": {
    "os": "macOS Sequoia",
    "shell": "zsh",
    "editor": "neovim",
    "terminal": "WezTerm + tmux",
    "package_manager": "homebrew + mise",
    "dotfiles_repo": "cyperx84/dotfiles",
    "key_tools": ["fzf", "ripgrep", "bat", "delta", "starship"]
  }
}
```

---

### `soul-forge audit`

Checks your generated agent files for issues.

```bash
# Audit all agents
soul-forge audit --all

# Audit a specific agent
soul-forge audit --agent coder
```

Checks:
- Missing files (`SOUL.md`/`USER.md` error; `AGENTS.md`/`TOOLS.md`/`MEMORY.md` warn)
- Empty or placeholder sections, staleness (file older than `profile.json`)
- **SOUL.md quality:** persona sections present (believes / decides / won't do),
  vague-or-hedging language flagged, and a soft ~1500-word length ceiling

Exits with code `1` if errors or warnings are found (CI-friendly).

```bash
# Use in CI
soul-forge audit --all || echo "Agent files need updating"
```

---

## File Structure

```
your-project/
├── soul-forge.yaml          # Agent fleet config (+ per-agent personas)
├── .soul-forge/
│   ├── profile.json         # Your structured profile (about you)
│   └── dotfiles.json        # Extracted dotfiles info (optional)
└── agents/
    ├── assistant/
    │   ├── SOUL.md          # who the agent is (voice & stance)
    │   ├── IDENTITY.md      # name, vibe, emoji
    │   ├── USER.md          # who you are
    │   ├── AGENTS.md        # operating procedure (SOP + memory routine)
    │   ├── TOOLS.md         # local tools/env cheat-sheet
    │   └── MEMORY.md        # learned over time (preserved on regenerate)
    └── coder/
        └── …                # same six files
```

---

## Releasing

Releases are tag-driven. Pushing a `v*` tag runs [GoReleaser](https://goreleaser.com),
which builds the binaries, publishes the GitHub release, and updates the Homebrew
formula; a follow-up job publishes the npm launcher.

```bash
git tag v0.2.0 && git push origin v0.2.0
```

Required repo secrets: `TAP_TOKEN` (write access to `cyperx84/homebrew-tap`) and
`NPM_TOKEN` (npm publish). `GITHUB_TOKEN` is provided automatically.

---

## License

MIT — see [LICENSE](LICENSE).
