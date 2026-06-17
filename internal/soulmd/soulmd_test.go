package soulmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "SOUL.md"), `# Ada

A one-line summary.

## Who I Am

A pragmatic engineer who has shipped a lot.

## Worldview

- Simple beats clever.
- Ship small.

## Opinions

### On testing

- Tests ride with the change.

## Tensions & Contradictions

- I value speed but won't ship what I can't verify.

## Boundaries

- I won't fabricate results.

## Pet Peeves

- Filler preamble.
`)
	writeFile(t, filepath.Join(dir, "STYLE.md"), `## Voice Principles

Dry, precise, allergic to filler.

## Anti-Patterns

- No corporate hedging.
`)
	examplesDir := filepath.Join(dir, "examples")
	if err := os.MkdirAll(examplesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(examplesDir, "good-outputs.md"), `### Adding retries

> Done — 3-attempt backoff. The endpoint isn't idempotent though.
Calibration: leads with the caveat, not an apology.
`)
	writeFile(t, filepath.Join(examplesDir, "bad-outputs.md"), `### Over-eager

> Certainly! I'd be absolutely delighted to help with that!
Why: servile preamble, no substance.
`)

	p, err := Parse(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Backstory != "A pragmatic engineer who has shipped a lot." {
		t.Fatalf("backstory=%q", p.Backstory)
	}
	if p.Voice != "Dry, precise, allergic to filler." {
		t.Fatalf("voice=%q", p.Voice)
	}
	if len(p.Opinions) != 3 { // worldview (2) + opinions subsection (1)
		t.Fatalf("opinions=%v", p.Opinions)
	}
	if len(p.Tensions) != 1 || len(p.Boundaries) != 1 {
		t.Fatalf("tensions=%v boundaries=%v", p.Tensions, p.Boundaries)
	}
	if len(p.Avoid) != 2 { // pet peeve + style anti-pattern
		t.Fatalf("avoid=%v", p.Avoid)
	}
	if len(p.Examples) != 1 || p.Examples[0].Note != "leads with the caveat, not an apology." {
		t.Fatalf("examples=%+v", p.Examples)
	}
	if p.Examples[0].Prompt != "Adding retries" {
		t.Fatalf("example prompt=%q", p.Examples[0].Prompt)
	}
	if len(p.Counters) != 1 || p.Counters[0].Note != "servile preamble, no substance." {
		t.Fatalf("counters=%+v", p.Counters)
	}
}

func TestParseMissingSoul(t *testing.T) {
	if _, err := Parse(t.TempDir()); err == nil {
		t.Fatal("expected error when SOUL.md is missing")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
