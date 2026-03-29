# soul-forge

Scaffold and generate `USER.md` + `SOUL.md` files for [OpenClaw](https://openclaw.dev) agent fleets.

soul-forge handles the structured parts of agent personality/context setup — questionnaire templates, dotfiles extraction, file generation from structured profiles, and auditing existing files. **No LLM API calls** — pure structured I/O.

---

## Install

```bash
go install github.com/cyperx84/soul-forge@latest
```

Or build from source:

```bash
git clone https://github.com/cyperx84/soul-forge
cd soul-forge
go build -o soul-forge .
```

---

## Workflow

```
init → questions → (fill out answers) → import → generate → audit
```

1. **`soul-forge init`** — set up config in current directory
2. **`soul-forge questions`** — get the onboarding questionnaire
3. Fill out your answers, then build a `profile.json` from them
4. **`soul-forge import profile.json`** — load your profile
5. **`soul-forge generate --all`** — generate agent files
6. **`soul-forge audit --all`** — verify everything looks good

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
    role: software-engineer
    channel: dev
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

Generates `USER.md` and `SOUL.md` per agent from your profile and config.

```bash
# Generate for all agents
soul-forge generate --all

# Generate for a specific agent
soul-forge generate --agent coder

# Preview without writing files
soul-forge generate --all --dry-run

# Skip animation
soul-forge generate --all --no-animation
```

Output files are placed at `<output_dir>/<agent-name>/USER.md` and `SOUL.md`.

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
    "timezone": "Europe/London"
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
- Missing `USER.md` or `SOUL.md` files
- Empty or placeholder sections
- Staleness (file is older than `profile.json`)

Exits with code `1` if issues found (CI-friendly).

```bash
# Use in CI
soul-forge audit --all || echo "Agent files need updating"
```

---

## File Structure

```
your-project/
├── soul-forge.yaml          # Agent fleet config
├── .soul-forge/
│   ├── profile.json         # Your structured profile
│   └── dotfiles.json        # Extracted dotfiles info (optional)
└── agents/
    ├── assistant/
    │   ├── USER.md
    │   └── SOUL.md
    └── coder/
        ├── USER.md
        └── SOUL.md
```

---

## License

MIT
