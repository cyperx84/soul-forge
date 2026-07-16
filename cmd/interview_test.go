package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/ingest"
)

// The brief is template + run parameters + questions, and the template must be
// byte-identical for every user. These tests pin that boundary.

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

func renderBrief(t *testing.T, host string, agents []string) string {
	t.Helper()
	opts := ingest.Options{Host: host, Agents: agents}
	ps, batches := interviewBatches(t, opts)
	var sb strings.Builder
	if err := emitInterview(&sb, ps, batches, host, agents); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

// templatePart cuts the brief at the run-parameters boundary. Everything before
// it is the template; everything after is this run's data.
func templatePart(t *testing.T, brief string) string {
	t.Helper()
	idx := strings.Index(brief, "# Run parameters")
	if idx < 0 {
		t.Fatal("brief has no Run parameters block; the template/data boundary is gone")
	}
	return brief[:idx]
}

func TestInterviewTemplateIsIdenticalAcrossRuns(t *testing.T) {
	a := templatePart(t, renderBrief(t, "box-a", []string{"alpha", "beta"}))
	b := templatePart(t, renderBrief(t, "other-machine", []string{"gamma"}))
	c := templatePart(t, renderBrief(t, "", nil))

	// The generalization contract: two users with different machines, rosters,
	// and files get byte-identical interviewer instructions. Any interpolation
	// of run data into the template breaks this test by construction.
	if a != b || b != c {
		t.Error("template bytes differ across runs — run data leaked into the template")
	}
}

func TestInterviewTemplateNamesNoValues(t *testing.T) {
	tpl := templatePart(t, renderBrief(t, "box-a", []string{"alpha", "beta"}))

	// Not even this run's own values may appear in the template prose — they
	// belong in the Run parameters block, which the template points at.
	for _, v := range []string{"box-a", "alpha", "beta"} {
		if strings.Contains(tpl, v) {
			t.Errorf("template prose contains run value %q; it must only reference Run parameters", v)
		}
	}
}

func TestInterviewRunParametersCarryTheRunData(t *testing.T) {
	out := renderBrief(t, "box-a", []string{"alpha", "beta"})
	params := out[strings.Index(out, "# Run parameters"):]

	for _, want := range []string{"`box-a`", "`alpha`", "`beta`"} {
		if !strings.Contains(params, want) {
			t.Errorf("Run parameters missing %q", want)
		}
	}
}

func TestInterviewDefaultsWhenParametersAbsent(t *testing.T) {
	out := renderBrief(t, "", nil)

	if !strings.Contains(out, "Machine id:** not supplied") {
		t.Error("absent host must tell the interviewer to ask the user for a machine name")
	}
	if !strings.Contains(out, "Agent roster:** not supplied") {
		t.Error("absent roster must tell the interviewer to accept any agent name")
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
