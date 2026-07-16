package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/fragment"
	"github.com/cyperx84/soul-forge/internal/ingest"
)

// The onboarding brief holds the same template/data boundary as the tagging brief,
// and the answers path must produce a corpus loadCorpus accepts — those are the two
// contracts pinned here.

func onboardTemplatePart(t *testing.T, brief string) string {
	t.Helper()
	idx := strings.Index(brief, "# Run parameters")
	if idx < 0 {
		t.Fatal("onboarding brief has no Run parameters block; the template/data boundary is gone")
	}
	return brief[:idx]
}

func renderOnboardBrief(t *testing.T, ps []ingest.Proposal, host string, agents []string) string {
	t.Helper()
	var sb strings.Builder
	if err := emitOnboardBrief(&sb, ps, host, agents); err != nil {
		t.Fatal(err)
	}
	return sb.String()
}

func TestOnboardTemplateIsIdenticalAcrossRuns(t *testing.T) {
	a := onboardTemplatePart(t, renderOnboardBrief(t, nil, "box-a", []string{"alpha"}))
	b := onboardTemplatePart(t, renderOnboardBrief(t, nil, "other", []string{"gamma", "delta"}))
	c := onboardTemplatePart(t, renderOnboardBrief(t, nil, "", nil))

	if a != b || b != c {
		t.Error("onboarding template bytes differ across runs — run data leaked into the template")
	}
	for _, v := range []string{"box-a", "alpha"} {
		if strings.Contains(a, v) {
			t.Errorf("onboarding template prose contains run value %q", v)
		}
	}
}

func TestOnboardBriefCarriesObservationsOnlyWhenScanned(t *testing.T) {
	cold := renderOnboardBrief(t, nil, "", nil)
	if !strings.Contains(cold, "No context files were scanned") {
		t.Error("context-free run must say observations are empty, not omit the block")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "NOTES.md")
	if err := os.WriteFile(path, []byte("# NOTES.md\n\n## Setup\n\n- Editor of choice is vim.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ps, err := proposeAll([]string{path}, ingest.Options{})
	if err != nil {
		t.Fatal(err)
	}
	warm := renderOnboardBrief(t, ps, "", nil)
	if !strings.Contains(warm, "Editor of choice is vim.") {
		t.Error("scanned run must carry the candidate lines into Observations")
	}
	if !strings.Contains(warm, "never to copy silently") {
		t.Error("observations must be framed as proposals to confirm, not defaults")
	}
}

func TestOnboardAnswersRoundTripThroughLoadCorpus(t *testing.T) {
	log := `
Great conversation! Here is the final log:

F: Never push to main without asking. | host=any profile=any kind=rule
F1: The user's notes live in their vault. | host=any profile=user kind=fact
F2: Speak tersely; skip filler. | host=any profile=alpha harness=any kind=voice
UNRESOLVED: "the two build agents but not the chat one" (note: sub-fleet scope)
PARKING: revisit backup strategy
`
	dir := t.TempDir()
	logPath := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(logPath, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}

	corpusPath := filepath.Join(dir, "corpus.json")
	out, err := os.Create(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	n, err := onboardCorpus(out, logPath)
	out.Close()
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("want 3 fragments, got %d", n)
	}

	// The output must be directly consumable by the same loader apply/compile use.
	// A corpus that needs hand-massaging between onboard and compile is a pipeline
	// with a human-shaped hole in it.
	frags, err := loadCorpus(corpusPath)
	if err != nil {
		t.Fatalf("loadCorpus rejected onboard output: %v", err)
	}
	if frags[0].Lifecycle != fragment.LifecycleAuthored {
		t.Error("interview fragments are authored doctrine")
	}
	if frags[2].Profile != "alpha" || frags[2].Kind != fragment.KindVoice {
		t.Errorf("tags not carried: %+v", frags[2])
	}
}

func TestOnboardAnswersRejectBadLinesByNumber(t *testing.T) {
	cases := map[string]string{
		"missing separator": "F: no tags here at all\n",
		"bad kind":          "F: text. | host=any profile=any kind=persona\n",
		"unknown tag":       "F: text. | host=any profile=any kind=rule mood=happy\n",
	}
	for name, log := range cases {
		dir := t.TempDir()
		path := filepath.Join(dir, "log.txt")
		if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := onboardCorpus(&strings.Builder{}, path)
		if err == nil {
			t.Errorf("%s: parse accepted a bad line", name)
			continue
		}
		if !strings.Contains(err.Error(), "line 1") {
			t.Errorf("%s: error does not name the line: %v", name, err)
		}
	}
}

func TestOnboardIDCollisionsSuffixDeterministically(t *testing.T) {
	log := "F: Never push to main. | host=any profile=any kind=rule\n" +
		"F: Never push to main. | host=any profile=alpha kind=rule\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	if _, err := onboardCorpus(&sb, path); err != nil {
		t.Fatal(err)
	}
	var frags []fragment.Fragment
	if err := json.Unmarshal([]byte(sb.String()), &frags); err != nil {
		t.Fatal(err)
	}
	if frags[0].ID == frags[1].ID {
		t.Errorf("colliding slugs must be suffixed, both got %q", frags[0].ID)
	}
}

func TestOnboardEmptyLogIsAnErrorNotAnEmptyCorpus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte("nice chat, no fragments\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := onboardCorpus(&strings.Builder{}, path); err == nil {
		t.Error("a log with no F lines must error — an empty corpus compiled downstream would gut real files")
	}
}
