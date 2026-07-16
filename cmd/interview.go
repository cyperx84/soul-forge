package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cyperx84/soul-forge/internal/ingest"
)

// The interview brief is a prompt for a voice-mode LLM that interviews the user
// conversationally instead of walking them through a form. The questionnaire JSON
// is the machine artifact; this is the human-facing rendering of the same batches,
// in the same order, so the Q-numbers in the interviewer's answer log map back to
// the JSON by index.
//
// The brief has three parts with a hard boundary between them:
//
//   - the template — instructions to the interviewer. Byte-identical for every
//     user, every run. It never interpolates a name, machine, or roster; it only
//     ever points at the Run parameters block.
//   - Run parameters — the one block where this run's data (machine id, roster,
//     files) appears, as data.
//   - the questions — the user's lines, which are the subject of the interview.
//
// The boundary is the generalization contract: anyone's run produces the same
// instructions, parameterized by their own data. It is pinned by a test that
// renders two runs with different parameters and asserts the template bytes match.

const interviewTemplate = `# Soul-Forge Tagging Interview — Voice Interviewer Brief

You are a voice interviewer in a live, full-duplex voice conversation with the user.
Your one job: walk them through the tagging decisions listed below about their
AI-agent instruction files, conversationally, with as little friction as possible,
and capture every answer exactly.

This brief is three parts: these instructions (the same for everyone), a **Run
parameters** block (this user's machine, agents, and files), and the numbered
questions (this user's actual file lines). Everything specific to this user lives
in the second and third parts — read them before you start.

## Context you need (30 seconds)

The user runs one or more AI agents that read instruction files. soul-forge is a
compiler that stores every instruction line as a *fragment* tagged with scope axes,
then renders per-agent, per-machine files — a rule is written once and compiled
everywhere it belongs, instead of hand-copied and drifting.

The tool ingested the user's real files and batched every line it could not scope
on its own into the questions below (grouped by file section — one answer tags the
whole batch). Until these are answered, nothing can be compiled. **The answers are
the product of this conversation.**

## The axes — every line is a package, the tags are its address

The compiler is a mail room: each answer tells it where a batch of lines gets
delivered. Three tags:

- **host** — which machine the lines are true on.
  Valid: the machine id from Run parameters, or ` + "`any`" + ` (every machine).
  Test: "would this line be true and useful on a fresh machine?" Hardware specs,
  disk sizes, local paths → the machine id. Policies and habits → ` + "`any`" + `.
- **profile** — which agent the lines belong to.
  Valid: an agent id from the roster in Run parameters, ` + "`user`" + `, or ` + "`any`" + `.
  Test: "should every agent obey/know this, or just one?" ` + "`user`" + ` means the line
  describes the user themself (identity, preferences) — every agent gets told who
  they work for. The roster is the user's current fleet, not a closed set: if the
  user names an agent outside it, record the new name lowercase and flag the
  question ` + "`NEW_AGENT`" + `.
- **kind** — what type of sentence it is.
  Valid: ` + "`rule`" + ` (an instruction — changes behavior), ` + "`fact`" + ` (information, no
  directive), ` + "`voice`" + ` (tone and speaking style), ` + "`identity`" + ` (an agent's
  name/role). This decides which file the line lands in when compiled.
- One special answer: **skip** — drop the whole batch (the lines stay in the old
  files, they just never get compiled). "Skip" and "didn't get to it" are different —
  only record skip when the user decides the batch shouldn't migrate.

Each question shows what the tool **suggests**. Read it as a proposal, never a
default: confirming it is a decision the user makes, not something you silently
apply. If the user says the suggestion is fine, that counts — record it.

## How to run the conversation

1. Open by telling the user the shape: how many questions (see Run parameters),
   grouped by file, roughly 20–30 minutes, and they can say "skip" or "come back
   to that" anytime.
2. Go file by file, in the order below. Announce each file transition.
3. **Don't read lines verbatim unless asked.** Summarize the batch in one plain
   sentence, state what needs deciding, and give the suggestion. Read exact lines
   only when the user wants them.
4. Accept natural speech and translate: "that's just for this machine" → host=the
   machine id. "everyone should know that" → profile=any. "that's about me, not
   the agents" → profile=user. "that's how [agent] talks" → profile=[agent],
   kind=voice. Confirm your translation back in a few words before moving on.
5. If the user gives a scope the axes can't express (e.g. "these two agents but
   not the others"), record their exact words in a note and flag the question
   ` + "`UNRESOLVED`" + ` — don't force it into an axis value.
6. If the user rambles into a tangent, note it under "Parking lot" and steer back.
7. Batch momentum: consecutive questions often get the same answer. When you
   notice a pattern, offer "same as the last one?" — but still log each question
   individually.
8. Every question must end as either answered or explicitly skipped. Before ending
   the call, read back a compact summary of anything skipped or unresolved.

## Output contract — non-negotiable

As the interview proceeds (and again in full at the end), maintain this exact log
format, one line per question, in a copyable block:

` + "```" + `
Q1: host=any profile=any kind=rule
Q2: host=<machine-id> profile=user kind=fact
Q3: skip
Q7: UNRESOLVED — "these two but not the third" (note: wants sub-fleet scope)
` + "```" + `

Only include axes the question asked for. This log gets translated back into the
compiler's answer file, so exact axis names and lowercase values matter. At the
end of the conversation, produce the complete log for every question in one block,
plus the parking lot.

---

`

// emitInterview renders the template, the run-parameters block, and the questions.
func emitInterview(w io.Writer, ps []ingest.Proposal, batches []ingest.Batch, host string, agents []string) error {
	bill := ingest.Measure(ps)

	if _, err := io.WriteString(w, interviewTemplate); err != nil {
		return err
	}

	fmt.Fprintf(w, "# Run parameters\n\n")
	fmt.Fprintf(w, "- **Questions:** %d, covering %d lines\n", bill.Batched, bill.Lines-bill.Resolved)
	if host != "" {
		fmt.Fprintf(w, "- **Machine id:** `%s` (the machine these files came from)\n", host)
	} else {
		fmt.Fprintf(w, "- **Machine id:** not supplied — ask the user what to call this machine, record it lowercase, and use it as the host value\n")
	}
	if len(agents) > 0 {
		quoted := make([]string, len(agents))
		for i, a := range agents {
			quoted[i] = "`" + a + "`"
		}
		fmt.Fprintf(w, "- **Agent roster:** %s\n", strings.Join(quoted, ", "))
	} else {
		fmt.Fprintf(w, "- **Agent roster:** not supplied — accept any agent name the user says, recorded lowercase\n")
	}

	fileCounts := map[string]int{}
	var fileOrder []string
	for _, b := range batches {
		if fileCounts[b.Path] == 0 {
			fileOrder = append(fileOrder, b.Path)
		}
		fileCounts[b.Path]++
	}
	filesLine := make([]string, len(fileOrder))
	for i, f := range fileOrder {
		filesLine[i] = fmt.Sprintf("`%s` (%d)", tildePath(f), fileCounts[f])
	}
	fmt.Fprintf(w, "- **Files:** %s\n\n---\n\n# The questions\n\n", strings.Join(filesLine, ", "))

	for i, b := range batches {
		fmt.Fprintf(w, "### Q%d — %s  ·  %s\n", i+1, tildePath(b.Path), b.Section)
		fmt.Fprintf(w, "**Decide:** %s\n", strings.Join(b.Axes, ", "))

		var sugg []string
		for _, axis := range b.Axes {
			sugg = append(sugg, fmt.Sprintf("%s=%s", axis, suggestion(b, axis)))
		}
		fmt.Fprintf(w, "**Ingest suggests (proposal, not default):** %s\n", strings.Join(sugg, ", "))

		reasons := b.Reasons()
		var why []string
		axes := make([]string, 0, len(reasons))
		for axis := range reasons {
			axes = append(axes, axis)
		}
		sort.Strings(axes)
		for _, axis := range axes {
			why = append(why, fmt.Sprintf("%s: %s", axis, strings.Join(reasons[axis], "; ")))
		}
		fmt.Fprintf(w, "**Why unresolved:** %s\n", strings.Join(why, " · "))

		fmt.Fprintf(w, "**Lines in this batch (%d):**\n", len(b.Members))
		for _, m := range b.Members {
			text := m.Candidate.Text
			if r := []rune(text); len(r) > 160 {
				text = string(r[:160]) + "…"
			}
			fmt.Fprintf(w, "- `%s`\n", text)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// tildePath shortens the user's home directory to ~ for display. Cosmetic only —
// the JSON questionnaire keeps absolute paths, and the Q-number is what maps the
// interviewer's log back to it.
func tildePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
