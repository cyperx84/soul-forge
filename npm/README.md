# soul-forge (npm)

npm/npx distribution of [**soul-forge**](https://github.com/cyperx84/soul-forge) — a
CLI that generates `SOUL.md`, `IDENTITY.md`, `USER.md`, `AGENTS.md`, `TOOLS.md`, and
`MEMORY.md` files for AI agent fleets (OpenClaw, Hermes, or any harness that reads a
soul.md).

```bash
# Run without installing
npx soul-forge --help

# Or install globally
npm install -g soul-forge
soul-forge init
```

This package is a thin launcher: on first run it downloads the matching prebuilt
binary for your platform (darwin/linux, amd64/arm64) from the GitHub release and execs
it. No build step, no Go toolchain required.

For other install methods (Homebrew, `go install`) and full usage, see the
[main README](https://github.com/cyperx84/soul-forge#readme).

## License

MIT
