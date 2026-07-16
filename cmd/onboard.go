package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cyperx84/soul-forge/internal/fragment"
	"github.com/cyperx84/soul-forge/internal/ingest"
	"github.com/spf13/cobra"
)

// onboard is the greenfield door. review/merge migrate files that exist; onboard
// authors a corpus for a user who has nothing — or nothing worth migrating.
//
// The shape matches the tagging interview deliberately: the CLI emits a brief for
// a voice-mode LLM, the human talks, the answer log comes back, the CLI turns it
// into fragments. The CLI never reasons about the user; the voice model does. Both
// doors converge on the same corpus format, so a blank-slate user and a migrating
// user end at identical state.
//
// Optional context paths are pre-fill: ingest reads them and the brief carries the
// observations, so the interviewer confirms ("looks like you use X — right?")
// instead of asking everything cold. Confirming a machine's observation is still
// the user's decision — the observation block says so in as many words.

var (
	onboardHost    string
	onboardAgents  []string
	onboardAnswers string
	onboardOutput  string
)

var onboardCmd = &cobra.Command{
	Use:   "onboard [context-path...]",
	Short: "Author a fragment corpus from a live voice interview",
	Long: `Emits an onboarding brief for a voice-mode LLM that interviews the user from
scratch: who they are, their agents, machines, red lines, voice, and working style.

Optional context paths (dotfiles, notes, existing configs) are ingested and carried
into the brief as observations for the interviewer to confirm, so the conversation
starts warm instead of cold.

With --answers it reads the interviewer's fragment log back and emits a validated
fragment corpus (a JSON array, ready for compile/apply --corpus).`,
	RunE: runOnboard,
}

func init() {
	rootCmd.AddCommand(onboardCmd)
	onboardCmd.Flags().StringVar(&onboardHost, "host", "", "machine id the user's primary machine should compile for")
	onboardCmd.Flags().StringSliceVar(&onboardAgents, "agents", nil, "agent ids the user already knows they want")
	onboardCmd.Flags().StringVar(&onboardAnswers, "answers", "", "path to the interviewer's fragment log; emits the corpus instead of the brief")
	onboardCmd.Flags().StringVar(&onboardOutput, "out", "", "write output here instead of stdout")
}

func runOnboard(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()
	if onboardOutput != "" {
		f, err := os.Create(onboardOutput)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}

	if onboardAnswers != "" {
		if len(args) > 0 {
			return fmt.Errorf("--answers consumes a log; context paths belong to the brief-emitting run")
		}
		n, err := onboardCorpus(w, onboardAnswers)
		if err != nil {
			return err
		}
		if onboardOutput != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "%d fragments → %s\n", n, onboardOutput)
		}
		return nil
	}

	var proposals []ingest.Proposal
	if len(args) > 0 {
		files, err := expand(args)
		if err != nil {
			return err
		}
		proposals, err = proposeAll(files, ingest.Options{Host: onboardHost, Agents: onboardAgents})
		if err != nil {
			return err
		}
	}
	if err := emitOnboardBrief(w, proposals, onboardHost, onboardAgents); err != nil {
		return err
	}
	if onboardOutput != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "onboarding brief → %s\n", onboardOutput)
	}
	return nil
}

// The onboarding template holds the same hard boundary as the tagging brief:
// byte-identical instructions for every user, all run data in Run parameters and
// the observations block. Same pinning test, same reason.
const onboardTemplate = `# Soul-Forge Onboarding Interview — Voice Interviewer Brief

You are a voice interviewer in a live, full-duplex voice conversation with the user.
Your job: draw out of them everything their AI agents need to know — who they are,
what agents they want, on which machines, with what rules, boundaries, and voice —
and author it as tagged one-sentence fragments. There are no pre-written questions
to march through; the section framework below is your map, the conversation is
yours to steer.

This brief is three parts: these instructions (the same for everyone), a **Run
parameters** block (what is already known about this user's setup), and an
**Observations** block (what a scan of their existing files suggested — possibly
empty). Everything specific to this user lives in the last two parts.

## What you are producing

soul-forge is a compiler. It stores every instruction as a *fragment* — one
sentence, tagged with where it applies — and renders each agent's instruction
files from them. A rule is written once and compiled everywhere it belongs.
Your interview authors those fragments. Each one carries:

- **host** — which machine it is true on: a machine id, or ` + "`any`" + `.
- **profile** — which agent it belongs to: an agent id, ` + "`user`" + ` (describes the
  human — every agent gets told who they work for), or ` + "`any`" + ` (every agent).
- **harness** — which agent runtime it is specific to: ` + "`openclaw`" + `, ` + "`claude`" + `,
  ` + "`hermes`" + `, ` + "`codex`" + `, or ` + "`any`" + `. When in doubt, ` + "`any`" + `.
- **kind** — what it is: ` + "`rule`" + ` (an instruction — changes behavior), ` + "`fact`" + `
  (information), ` + "`voice`" + ` (tone and speaking style), ` + "`identity`" + ` (an agent's
  name and role).

## The section framework

Work through these areas in whatever order the conversation makes natural, but
cover them all before ending:

1. **The user.** Name, what to call them, pronouns if offered, timezone. What they
   do, what they care about, how deep their technical background runs. → ` + "`user`" + `
   fragments, kind ` + "`fact`" + `.
2. **The roster.** Which agents exist or should exist, each one's name and job in
   one sentence. → ` + "`identity`" + ` fragments, one per agent.
3. **Machines.** What boxes the agents run on, what is specific to each (hardware
   limits, disk pressure, OS quirks, sacred directories). → host-tagged ` + "`fact`" + `
   fragments.
4. **Red lines.** What agents must never do: data that never leaves the machine,
   files never touched, commands never run, things never bought or sent. Probe —
   users forget these until asked. → ` + "`rule`" + ` fragments, usually profile ` + "`any`" + `.
5. **Working style.** How they want agents to operate: ask-first vs act-first,
   how much to confirm, how to handle ambiguity, what "done" means to them.
   → ` + "`rule`" + ` fragments.
6. **Voice.** How agents should sound: terse or warm, formal or casual, opinions
   or neutrality, things that annoy them in AI writing. → ` + "`voice`" + ` fragments,
   per agent or ` + "`any`" + `.
7. **Tools and memory.** Anything they already know they want remembered or
   integrated: note systems, vaults, messaging, install policies. → ` + "`rule`" + ` and
   ` + "`fact`" + ` fragments.

## How to run the conversation

1. Open with the shape: a conversation, not a form — roughly 20–30 minutes, seven
   areas, they can wander and you will keep the map.
2. If Observations below are non-empty, confirm them early instead of asking cold
   ("your files suggest you use X — still true?"). A confirmed observation becomes
   a fragment like any other; an observation is a proposal, never a default.
3. One idea per fragment. When the user gives a paragraph, split it into single
   sentences and read the important ones back for confirmation.
4. Author fragment text in second person imperative for rules ("Never push to
   main without asking"), plain declaratives for facts ("The user's notes live in
   Obsidian"). No hedging words.
5. Tag as you go, out loud only when uncertain. "That sounds machine-specific —
   just this box, or every box?" is a good question; reading tags aloud for every
   fragment is friction.
6. If the user names a scope the tags cannot express (two agents but not a third),
   record their exact words and flag the line ` + "`UNRESOLVED`" + `.
7. Tangents: note them under "Parking lot" and steer back.
8. Before ending: read back a one-minute summary per section, then ask the two
   questions users answer best last — "what would make you turn this whole thing
   off?" and "what do you wish an assistant already knew about you?"

## Output contract — non-negotiable

As the interview proceeds (and again in full at the end), maintain a log: one line
per fragment, in a copyable block, exactly this shape:

` + "```" + `
F: <one-sentence fragment text> | host=any profile=user kind=fact
F: <one-sentence fragment text> | host=<machine-id> profile=any kind=rule
F: <text> | host=any profile=<agent> harness=any kind=voice
UNRESOLVED: "<the user's exact words>" (note: <why the tags cannot express it>)
PARKING: <tangent worth returning to>
` + "```" + `

Rules for the log: the ` + "` | `" + ` separator is required; host, profile, and kind are
required on every F line; harness is optional and defaults to ` + "`any`" + `; values are
lowercase. This log is parsed by a compiler — exact key names matter, prose
outside the log lines does not. At the end, produce the complete log in one block.

---

`

// emitOnboardBrief renders the template, run parameters, and any observations.
func emitOnboardBrief(w io.Writer, ps []ingest.Proposal, host string, agents []string) error {
	if _, err := io.WriteString(w, onboardTemplate); err != nil {
		return err
	}

	fmt.Fprintf(w, "# Run parameters\n\n")
	if host != "" {
		fmt.Fprintf(w, "- **Machine id:** `%s`\n", host)
	} else {
		fmt.Fprintf(w, "- **Machine id:** not supplied — ask the user what to call their primary machine, record it lowercase, and use it as the host value\n")
	}
	if len(agents) > 0 {
		quoted := make([]string, len(agents))
		for i, a := range agents {
			quoted[i] = "`" + a + "`"
		}
		fmt.Fprintf(w, "- **Agent roster:** %s (a starting point — the roster section may grow or shrink it)\n", strings.Join(quoted, ", "))
	} else {
		fmt.Fprintf(w, "- **Agent roster:** none yet — the roster section of the interview defines it\n")
	}

	fmt.Fprintf(w, "\n---\n\n# Observations\n\n")
	if len(ps) == 0 {
		fmt.Fprintf(w, "No context files were scanned for this run. Everything comes from the conversation.\n")
		return nil
	}

	fmt.Fprintf(w, "A scan of the user's existing files produced these candidate lines. Each is a\nproposal to confirm, rephrase, or discard with the user — never to copy silently.\n\n")
	byFile := map[string][]ingest.Proposal{}
	var order []string
	for _, p := range ps {
		path := p.Candidate.Path
		if len(byFile[path]) == 0 {
			order = append(order, path)
		}
		byFile[path] = append(byFile[path], p)
	}
	sort.Strings(order)
	for _, path := range order {
		fmt.Fprintf(w, "### %s\n", tildePath(path))
		for _, p := range byFile[path] {
			text := p.Candidate.Text
			if r := []rune(text); len(r) > 160 {
				text = string(r[:160]) + "…"
			}
			fmt.Fprintf(w, "- `%s`\n", text)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// onboardCorpus parses the interviewer's fragment log into a validated corpus and
// writes it as a JSON array — the exact shape loadCorpus reads, so the output feeds
// compile/apply --corpus with no massaging.
func onboardCorpus(w io.Writer, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("answers: %w", err)
	}
	defer f.Close()

	var frags []fragment.Fragment
	seen := map[string]int{}
	var unresolved int

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "UNRESOLVED:"):
			// Counted, not dropped silently: an unresolved scope is work the corpus
			// still owes, and a parse that swallowed it would report clean.
			unresolved++
			continue
		case !strings.HasPrefix(line, "F"):
			continue
		}
		rest, ok := strings.CutPrefix(line, "F")
		if !ok {
			continue
		}
		rest = strings.TrimLeft(rest, "0123456789")
		rest, ok = strings.CutPrefix(rest, ":")
		if !ok {
			continue // "Fragments were..." prose, not a log line
		}

		frag, err := parseFragmentLine(rest)
		if err != nil {
			return 0, fmt.Errorf("answers line %d: %w", lineNo, err)
		}

		// IDs are generated, not authored, so a collision is two similar sentences —
		// suffix deterministically rather than stop. The duplicate-ID invariant guards
		// authored corpora, where a collision means two definitions of one rule.
		base := slugText(frag.Text)
		seen[base]++
		if n := seen[base]; n > 1 {
			frag.ID = fmt.Sprintf("%s-%d", base, n)
		} else {
			frag.ID = base
		}

		if err := frag.Validate(); err != nil {
			return 0, fmt.Errorf("answers line %d: %w", lineNo, err)
		}
		frags = append(frags, frag)
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	if len(frags) == 0 {
		return 0, fmt.Errorf("answers %s: no F lines found — is this the interviewer's log?", path)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(frags); err != nil {
		return 0, err
	}
	if unresolved > 0 {
		fmt.Fprintf(os.Stderr, "note: %d UNRESOLVED line(s) in the log — scopes the axes could not express, not yet in the corpus\n", unresolved)
	}
	return len(frags), nil
}

// parseFragmentLine splits "<text> | key=value ..." into a fragment. The last " | "
// wins, so fragment text may itself contain pipes.
func parseFragmentLine(s string) (fragment.Fragment, error) {
	idx := strings.LastIndex(s, " | ")
	if idx < 0 {
		return fragment.Fragment{}, fmt.Errorf("missing ' | ' separator between text and tags")
	}
	text := strings.TrimSpace(s[:idx])
	if text == "" {
		return fragment.Fragment{}, fmt.Errorf("empty fragment text")
	}

	frag := fragment.Fragment{
		Text:      text,
		Harness:   fragment.AxisAny,
		Lifecycle: fragment.LifecycleAuthored,
	}
	for _, tok := range strings.Fields(s[idx+3:]) {
		key, value, ok := strings.Cut(tok, "=")
		if !ok {
			return fragment.Fragment{}, fmt.Errorf("tag %q is not key=value", tok)
		}
		value = strings.ToLower(strings.TrimSpace(value))
		switch key {
		case "host":
			frag.Host = value
		case "profile":
			frag.Profile = value
		case "harness":
			frag.Harness = value
		case "kind":
			frag.Kind = value
		default:
			return fragment.Fragment{}, fmt.Errorf("unknown tag %q (valid: host, profile, harness, kind)", key)
		}
	}
	return frag, nil
}

// slugText names a fragment from its opening words. Generated names are a floor,
// not a ceiling — the corpus owner renames what matters.
func slugText(text string) string {
	words := strings.Fields(strings.ToLower(text))
	if len(words) > 6 {
		words = words[:6]
	}
	slug := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r == ' ', r == '-', r == '_':
			return '-'
		}
		return -1
	}, strings.Join(words, " "))
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "fragment"
	}
	return slug
}
