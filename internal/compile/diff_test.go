package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

// driftCorpus is the same shape as the rest of the v2 test corpus: real lines from the
// hand-built rewrite, not invented fixtures. If a test only passes on made-up input it
// is testing the test.
func driftCorpus() []fragment.Fragment {
	return []fragment.Fragment{
		{
			ID: "trash-over-rm", Text: "`trash` > `rm`. Make destructive ops restorable.",
			Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
		},
		{
			ID: "klaw-orchestrates", Text: "Klaw orchestrates the fleet.",
			Host: fragment.AxisAny, Profile: "klaw", Harness: fragment.HarnessOpenClaw,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindIdentity,
		},
		{
			ID: "m4-disk-tight", Text: "Disk chronically tight (228GB, often 90%+).",
			Host: "m4-mini", Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindFact,
		},
		{
			ID: "lead-with-outcome", Text: "Lead with the outcome. Dense over verbose.",
			Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindVoice,
		},
		// Two more rules so AGENTS.md renders enough lines to reorder meaningfully.
		// A one-line file cannot express precedence, and TestDiffCatchesReorder needs a
		// file whose order carries meaning to prove that order is compared at all.
		{
			ID: "act-first", Text: "Act first, explain after. Routine ops need no preamble.",
			Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
		},
		{
			ID: "verify-premise", Text: "Verify the premise: reproduce the symptom before fixing it.",
			Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
			Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
		},
	}
}

func klawOnM4() Target {
	return Target{
		Name:     "openclaw-hub",
		Selector: fragment.Selector{Host: "m4-mini", Profile: "klaw", Harness: fragment.HarnessOpenClaw},
	}
}

// writeCompiled lays a target's compile output down on disk — the state right after an
// apply, which is the only state diff should ever call clean.
func writeCompiled(t *testing.T, corpus []fragment.Fragment, tgt Target) string {
	t.Helper()
	root := t.TempDir()
	res, err := Compile(corpus, tgt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for path, content := range res.Files {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root
}

func statusOf(t *testing.T, r Report, path string) string {
	t.Helper()
	for _, f := range r.Files {
		if f.Path == path {
			return f.Status
		}
	}
	t.Fatalf("path %q absent from report; report covered %v", path, reportPaths(r))
	return ""
}

func reportPaths(r Report) []string {
	var out []string
	for _, f := range r.Files {
		out = append(out, f.Path+"="+f.Status)
	}
	return out
}

// TestDiffCleanAfterApply pins the base case: compile, write, diff, silence. A drift
// detector that cries on a freshly applied tree is one nobody will run twice.
func TestDiffCleanAfterApply(t *testing.T) {
	corpus, tgt := driftCorpus(), klawOnM4()
	root := writeCompiled(t, corpus, tgt)

	report, err := Diff(corpus, tgt, root)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if report.HasDrift() {
		t.Fatalf("fresh apply reports drift: %v", reportPaths(report))
	}
	for _, f := range report.Files {
		if f.Status == StatusDrifted || f.Status == StatusMissing {
			t.Errorf("%s: status %q on a freshly applied tree", f.Path, f.Status)
		}
	}
}

// TestDiffCatchesHandEdit is the whole point of the tool. The inverse of
// TestDiffCleanAfterApply: if this passes while that one also passes, the comparison
// is real. A detector that reports clean unconditionally passes the clean test alone.
func TestDiffCatchesHandEdit(t *testing.T) {
	corpus, tgt := driftCorpus(), klawOnM4()
	root := writeCompiled(t, corpus, tgt)

	const smuggled = "- Ship it, skip the tests."
	agents := filepath.Join(root, "AGENTS.md")
	live, err := os.ReadFile(agents)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := os.WriteFile(agents, append(live, []byte("\n"+smuggled+"\n")...), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	report, err := Diff(corpus, tgt, root)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !report.HasDrift() {
		t.Fatalf("hand edit not reported as drift: %v", reportPaths(report))
	}
	if got := statusOf(t, report, "AGENTS.md"); got != StatusDrifted {
		t.Errorf("AGENTS.md status = %q, want %q", got, StatusDrifted)
	}

	// The explanation has to name the smuggled line, not just say "differs". A drift
	// report you have to re-derive by hand is the archaeology this tool exists to end.
	for _, f := range report.Files {
		if f.Path != "AGENTS.md" {
			continue
		}
		if !containsLine(f.OnlyLive, smuggled) {
			t.Errorf("OnlyLive = %q, want it to name %q", f.OnlyLive, smuggled)
		}
		if len(f.OnlyCompiled) != 0 {
			t.Errorf("OnlyCompiled = %q, want empty: nothing was removed", f.OnlyCompiled)
		}
	}
}

// TestDiffCatchesDeletion covers the other direction: a red line quietly deleted from a
// live file is exactly the drift with teeth, and it must not read as clean.
func TestDiffCatchesDeletion(t *testing.T) {
	corpus, tgt := driftCorpus(), klawOnM4()
	root := writeCompiled(t, corpus, tgt)

	agents := filepath.Join(root, "AGENTS.md")
	live, err := os.ReadFile(agents)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var kept []string
	for _, line := range strings.Split(string(live), "\n") {
		if strings.Contains(line, "trash") {
			continue
		}
		kept = append(kept, line)
	}
	if err := os.WriteFile(agents, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	report, err := Diff(corpus, tgt, root)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !report.HasDrift() {
		t.Fatal("a deleted red line reports clean")
	}
	for _, f := range report.Files {
		if f.Path == "AGENTS.md" && !anyContains(f.OnlyCompiled, "trash") {
			t.Errorf("OnlyCompiled = %q, want it to name the deleted trash > rm line", f.OnlyCompiled)
		}
	}
}

// TestDiffSkeletonIsNeverDrift pins invariant 3. MEMORY.md holds lifecycle:instance
// content — the runtime writes it, compile owns only its existence. Real accreted
// memory diverging from the scaffold is correct, and calling it drift would either
// train the operator to ignore the detector or invite an apply that eats real memory.
func TestDiffSkeletonIsNeverDrift(t *testing.T) {
	corpus, tgt := driftCorpus(), klawOnM4()
	root := writeCompiled(t, corpus, tgt)

	memory := filepath.Join(root, "MEMORY.md")
	if _, err := os.Stat(memory); err != nil {
		t.Fatalf("compile did not scaffold MEMORY.md: %v", err)
	}
	runtimeWritten := "# MEMORY.md\n\n- Fleet vault archived 2026-07-13; don't rebuild without direction.\n"
	if err := os.WriteFile(memory, []byte(runtimeWritten), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	report, err := Diff(corpus, tgt, root)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if got := statusOf(t, report, "MEMORY.md"); got != StatusSkeleton {
		t.Errorf("MEMORY.md status = %q, want %q: runtime memory is not the compiler's business", got, StatusSkeleton)
	}
	if report.HasDrift() {
		t.Errorf("runtime-written memory reported as drift: %v", reportPaths(report))
	}
}

// TestDiffSkeletonMissingIsDrift is the inverse of the above, and the reason skeleton
// immunity does not mean skeleton invisibility: compile owns the file's existence, so
// an absent MEMORY.md is a real finding even though its contents never are.
func TestDiffSkeletonMissingIsDrift(t *testing.T) {
	corpus, tgt := driftCorpus(), klawOnM4()
	root := writeCompiled(t, corpus, tgt)

	if err := os.Remove(filepath.Join(root, "MEMORY.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	report, err := Diff(corpus, tgt, root)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if got := statusOf(t, report, "MEMORY.md"); got != StatusMissing {
		t.Errorf("absent MEMORY.md status = %q, want %q", got, StatusMissing)
	}
	if !report.HasDrift() {
		t.Error("absent skeleton file reports clean")
	}
}

// TestDiffCatchesReorder proves status is decided on bytes, not on a line set. A
// line-set comparison is a tempting shortcut and it would call this clean — reordering
// a rules file changes what an agent reads first, and precedence is meaning.
func TestDiffCatchesReorder(t *testing.T) {
	corpus, tgt := driftCorpus(), klawOnM4()
	root := writeCompiled(t, corpus, tgt)

	res, err := Compile(corpus, tgt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	compiled := res.Files["AGENTS.md"]
	split := strings.Split(strings.TrimRight(compiled, "\n"), "\n")
	// Fatal, not Skip. A skip here silently disables the only test that pins
	// bytes-not-line-sets, and it reports ok while doing it — which is exactly what
	// happened on the first draft of this file. A test that cannot test must shout.
	if len(lines(compiled)) < 3 {
		t.Fatalf("corpus renders %d content lines into AGENTS.md; this test needs >=3 to reorder meaningfully", len(lines(compiled)))
	}
	reversed := make([]string, len(split))
	for i, l := range split {
		reversed[len(split)-1-i] = l
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(strings.Join(reversed, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	report, err := Diff(corpus, tgt, root)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !report.HasDrift() {
		t.Fatal("a reordered file reports clean: status is being decided on a line set, not bytes")
	}
	for _, f := range report.Files {
		if f.Path != "AGENTS.md" {
			continue
		}
		if !f.Reordered {
			t.Errorf("Reordered = false; OnlyCompiled=%q OnlyLive=%q — same lines, different order should say so",
				f.OnlyCompiled, f.OnlyLive)
		}
	}
}

// TestDiffMissingTree is the fresh-machine case: nothing applied yet. Everything
// compile owns is missing, and that is drift — it is precisely what `apply` fixes.
func TestDiffMissingTree(t *testing.T) {
	corpus, tgt := driftCorpus(), klawOnM4()
	report, err := Diff(corpus, tgt, t.TempDir())
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !report.HasDrift() {
		t.Fatal("empty tree reports clean")
	}
	for _, f := range report.Files {
		if f.Status != StatusMissing {
			t.Errorf("%s: status %q on an empty tree, want %q", f.Path, f.Status, StatusMissing)
		}
	}
	if got := statusOf(t, report, "AGENTS.md"); got != StatusMissing {
		t.Errorf("AGENTS.md = %q, want %q", got, StatusMissing)
	}
}

// TestDiffUnmanagedIsNotDrift covers a file the render map names that compile emitted
// nothing for — a retired fragment's leftover. Reported, because a silent leftover is
// how a stale rule keeps being injected; not drift, because the tool has no basis to
// call the operator's leftover wrong.
func TestDiffUnmanagedIsNotDrift(t *testing.T) {
	corpus, tgt := driftCorpus(), klawOnM4()
	root := writeCompiled(t, corpus, tgt)

	// HEARTBEAT.md is in the render map's Order and Skeleton; USER.md is in Order and
	// this corpus has no profile:user fragment, so compile emits nothing for it.
	stale := filepath.Join(root, "USER.md")
	if _, err := os.Stat(stale); err == nil {
		t.Fatal("corpus emits USER.md; this test needs a render-map path compile leaves empty — fix the corpus, don't skip")
	}
	if err := os.WriteFile(stale, []byte("# USER.md\n\n- left over from a retired fragment\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	report, err := Diff(corpus, tgt, root)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if got := statusOf(t, report, "USER.md"); got != StatusUnmanaged {
		t.Errorf("USER.md status = %q, want %q", got, StatusUnmanaged)
	}
	if report.HasDrift() {
		t.Errorf("an unmanaged leftover reported as drift: %v", reportPaths(report))
	}
}

// TestDiffUnreadableIsErrorNotMissing: a file we cannot read must not be reported as
// absent. Reporting "missing" would send apply to write over content it never saw —
// the detector failing in the one direction that destroys data.
func TestDiffUnreadableIsErrorNotMissing(t *testing.T) {
	corpus, tgt := driftCorpus(), klawOnM4()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "AGENTS.md"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, err := Diff(corpus, tgt, root)
	if err == nil {
		t.Fatal("an unreadable AGENTS.md produced no error; it would have been reported as missing")
	}
	if !strings.Contains(err.Error(), "AGENTS.md") {
		t.Errorf("error %q does not name the unreadable path", err)
	}
}

// TestDiffDeterministic pins report stability. diff is meant for cron and CI, where a
// report that reorders between runs is noise indistinguishable from a change.
func TestDiffDeterministic(t *testing.T) {
	corpus, tgt := driftCorpus(), klawOnM4()
	root := writeCompiled(t, corpus, tgt)

	first, err := Diff(corpus, tgt, root)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	for i := 0; i < 5; i++ {
		next, err := Diff(corpus, tgt, root)
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if len(next.Files) != len(first.Files) {
			t.Fatalf("run %d: %d files, first run had %d", i, len(next.Files), len(first.Files))
		}
		for j := range next.Files {
			if next.Files[j].Path != first.Files[j].Path || next.Files[j].Status != first.Files[j].Status {
				t.Fatalf("run %d position %d: %v, first run %v", i, j, next.Files[j], first.Files[j])
			}
		}
	}
}

// TestDiffNeverWrites: the detector is read-only. It runs on cron against live
// workspaces, so a write from a "just checking" command is the worst kind of surprise.
func TestDiffNeverWrites(t *testing.T) {
	corpus, tgt := driftCorpus(), klawOnM4()
	root := t.TempDir()

	if _, err := Diff(corpus, tgt, root); err != nil {
		t.Fatalf("Diff: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("Diff created %d entries in an empty root; it must never write", len(entries))
	}
}

// TestDiffRefusesBrokenCorpus: diff compiles first, so an invariant violation stops it
// rather than being reported as drift against whatever is on disk. A build that cannot
// compile has no compiled state to compare against.
func TestDiffRefusesBrokenCorpus(t *testing.T) {
	corpus := append(driftCorpus(), fragment.Fragment{
		ID: "leaked-key", Text: "Use sk-ant-api03-do-not-do-this for the fallback.",
		Host: fragment.AxisAny, Profile: fragment.AxisAny, Harness: fragment.AxisAny,
		Lifecycle: fragment.LifecycleAuthored, Kind: fragment.KindRule,
	})
	if _, err := Diff(corpus, klawOnM4(), t.TempDir()); err == nil {
		t.Fatal("diff on a corpus that fails an invariant returned no error")
	}
}

func containsLine(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}

func anyContains(lines []string, substr string) bool {
	for _, l := range lines {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}
