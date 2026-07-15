package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/ingest"
)

// The brief is a template the run parameterizes. The contract: every name in the
// output traces to a flag or an ingested file — the tool itself supplies none.

const interviewFixture = `# FILE.md

## Section One

- Machine has small disk.
- Always prefer careful tools.

## Section Two

- The orchestrator agent routes work.
`

func interviewBatches(t *testing.T, opts ingest.Options) ([]ingest.Proposal, []ingest.Batch) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "FILE.md")
	if err := os.WriteFile(path, []byte(interviewFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	ps, err := proposeAll([]string{path}, opts)
	if err != nil {
		t.Fatal(err)
	}
	return ps, ingest.Batches(ps)
}

func TestInterviewBriefCarriesNoNamesOfItsOwn(t *testing.T) {
	ps, batches := interviewBatches(t, ingest.Options{})
	var sb strings.Builder
	if err := emitInterview(&sb, ps, batches, "", nil); err != nil {
		t.Fatal(err)
	}
	out := sb.String()

	// Names from the author's own setup must never appear when flags didn't
	// supply them. This is the generalization contract, pinned.
	for _, leaked := range []string{"klaw", "Klaw", "builder", "researcher", "ops", "Chris", "m4-mini", "CLAUDE.md", "hermes"} {
		if strings.Contains(out, leaked) {
			t.Errorf("brief leaked author-specific name %q with no flags set", leaked)
		}
	}
	if !strings.Contains(out, "roster was not supplied") {
		t.Error("no-roster run must tell the interviewer to accept any agent name")
	}
}

func TestInterviewBriefUsesRunParameters(t *testing.T) {
	opts := ingest.Options{Host: "box-a", Agents: []string{"alpha", "beta"}}
	ps, batches := interviewBatches(t, opts)
	var sb strings.Builder
	if err := emitInterview(&sb, ps, batches, opts.Host, opts.Agents); err != nil {
		t.Fatal(err)
	}
	out := sb.String()

	for _, want := range []string{"`box-a`", "`alpha`", "`beta`", "NEW_AGENT"} {
		if !strings.Contains(out, want) {
			t.Errorf("brief missing run parameter %q", want)
		}
	}
}

func TestInterviewQuestionNumbersMatchQuestionnaireOrder(t *testing.T) {
	ps, batches := interviewBatches(t, ingest.Options{})
	if len(batches) == 0 {
		t.Fatal("fixture produced no batches; the test cannot test")
	}
	var sb strings.Builder
	if err := emitInterview(&sb, ps, batches, "", nil); err != nil {
		t.Fatal(err)
	}
	out := sb.String()

	// The interviewer's Q-log maps back to the JSON questionnaire by index, so the
	// brief must number batches in exactly the slice order the JSON uses.
	for i, b := range batches {
		header := "### Q" + strconv.Itoa(i+1) + " — "
		idx := strings.Index(out, header)
		if idx < 0 {
			t.Fatalf("brief missing question header %q", header)
		}
		rest := out[idx:]
		end := strings.Index(rest, "\n")
		if !strings.Contains(rest[:end], b.Section) {
			t.Errorf("Q%d header does not carry section %q: %q", i+1, b.Section, rest[:end])
		}
	}
}
