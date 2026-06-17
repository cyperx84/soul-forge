package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cyperx84/soul-forge/internal/voice"
	"github.com/spf13/cobra"
)

var voiceCmd = &cobra.Command{
	Use:   "voice <sample...>",
	Short: "Scan writing samples for voice signals (deterministic, no LLM)",
	Long: `Reads one or more writing samples (files or globs) and emits deterministic
stylometry — sentence rhythm, punctuation tics, hedging, distinctive vocabulary — to
.soul-forge/voice.json, with a "candidates" block of suggested persona.voice / .avoid /
vocabulary fields.

It proposes candidates; it never authors the persona. Surface stats overfit and models
can't reproduce implicit style, so the harness should present these to you for
confirmation before any land in soul-forge.yaml. Aim for 2000+ words across varied
samples (emails, messages, prose) — a single source yields a register, not a voice.

Privacy: only derived stats are written, never your raw text. Still, gitignore
.soul-forge/voice.json if your samples are personal.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runVoice,
}

func runVoice(cmd *cobra.Command, args []string) error {
	paths, err := expandGlobs(args)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no sample files matched %v", args)
	}

	var combined []byte
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		combined = append(combined, data...)
		combined = append(combined, '\n')
	}

	analysis := voice.Analyze(string(combined))
	fmt.Fprintf(os.Stderr, "Scanned %d file(s), %d words (confidence: %s)\n", len(paths), analysis.WordCount, analysis.Confidence)
	if analysis.Confidence == "low" {
		fmt.Fprintf(os.Stderr, "⚠ %s\n", analysis.Note)
	}

	if err := os.MkdirAll(".soul-forge", 0755); err != nil {
		return fmt.Errorf("create .soul-forge: %w", err)
	}
	f, err := os.Create(".soul-forge/voice.json")
	if err != nil {
		return fmt.Errorf("create voice.json: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(analysis); err != nil {
		return fmt.Errorf("write voice.json: %w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ Wrote .soul-forge/voice.json\n")

	out := json.NewEncoder(os.Stdout)
	out.SetIndent("", "  ")
	return out.Encode(analysis)
}

func expandGlobs(args []string) ([]string, error) {
	var paths []string
	for _, a := range args {
		matches, err := filepath.Glob(a)
		if err != nil {
			return nil, fmt.Errorf("bad pattern %q: %w", a, err)
		}
		if matches == nil {
			// Not a glob (or no match) — treat as a literal path so a clear
			// "read" error surfaces rather than silently dropping it.
			paths = append(paths, a)
			continue
		}
		paths = append(paths, matches...)
	}
	return paths, nil
}
