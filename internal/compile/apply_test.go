package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Apply is the first step that writes, so its tests are mostly about what it refuses
// to do. That creates a trap the diff tests already taught us: an implementation that
// writes nothing passes every "did not destroy" assertion and is worthless. So the
// destructive-safety tests below are paired with TestApplyActuallyWrites, which fails
// on exactly that no-op implementation.

func planFor(t *testing.T, root, backup string) ApplyPlan {
	t.Helper()
	p, err := Plan(driftCorpus(), klawOnM4(), root, backup)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	return p
}

// TestPlanNeverWrites pins the Plan/Commit type split. Plan is the step an operator
// runs to decide; if it can touch disk, the decision has already been made for them.
func TestPlanNeverWrites(t *testing.T) {
	root := t.TempDir()
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	p := planFor(t, root, "")
	if len(p.Changes()) == 0 {
		t.Fatal("empty root should plan creates; a plan with no changes here means Plan is not planning")
	}

	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("Plan wrote to disk: %d entries before, %d after", len(before), len(after))
	}
}

// TestApplyActuallyWrites is the inverse guard. Every safety test in this file passes
// trivially for a Commit that does nothing; this one does not.
func TestApplyActuallyWrites(t *testing.T) {
	root := t.TempDir()
	res, err := planFor(t, root, "").Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if len(res.Written) == 0 {
		t.Fatal("Commit wrote nothing to an empty root")
	}

	got, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md not written: %v", err)
	}
	if !strings.Contains(string(got), "trash") {
		t.Errorf("AGENTS.md missing its fragments:\n%s", got)
	}

	// And the result is convergent: applying twice is a no-op, because the second
	// diff reports clean. A tool that rewrites on every run churns mtimes and makes
	// its own drift signal untrustworthy.
	second := planFor(t, root, "")
	if len(second.Changes()) != 0 {
		t.Errorf("apply is not idempotent, second run plans: %v", second.Changes())
	}
}

// TestApplyNeverOverwritesSkeleton is invariant 3 at the write layer, and the single
// most destructive thing this tool could get wrong: MEMORY.md holds runtime-written
// memory that exists nowhere else. Compile owns its existence, never its contents.
func TestApplyNeverOverwritesSkeleton(t *testing.T) {
	root := t.TempDir()
	if _, err := planFor(t, root, "").Commit(); err != nil {
		t.Fatal(err)
	}

	const runtimeMemory = "# MEMORY.md\n\n- The M1 prime account was provisioned 2026-07-20.\n"
	mem := filepath.Join(root, "MEMORY.md")
	if err := os.WriteFile(mem, []byte(runtimeMemory), 0o644); err != nil {
		t.Fatal(err)
	}

	p := planFor(t, root, "")
	for _, f := range p.Files {
		if f.Path == "MEMORY.md" && f.Action != ActionNone {
			t.Fatalf("MEMORY.md planned for %q — runtime memory is not the compiler's business", f.Action)
		}
	}

	if _, err := p.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(mem)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != runtimeMemory {
		t.Fatalf("apply clobbered runtime memory:\nwant %q\ngot  %q", runtimeMemory, got)
	}
}

// TestApplyBacksUpBeforeOverwrite pins the entire mitigation for apply's one honest
// limitation: a drifted file is indistinguishable from a hand edit made five minutes
// ago, so the pre-write bytes must survive.
func TestApplyBacksUpBeforeOverwrite(t *testing.T) {
	root := t.TempDir()
	backup := filepath.Join(t.TempDir(), "backups")
	if _, err := planFor(t, root, backup).Commit(); err != nil {
		t.Fatal(err)
	}

	const handEdit = "# AGENTS.md\n\n- A rule Chris wrote by hand and never put in the corpus.\n"
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte(handEdit), 0o644); err != nil {
		t.Fatal(err)
	}

	p := planFor(t, root, backup)
	if !p.Destructive() {
		t.Fatal("plan overwriting a hand-edited AGENTS.md must report Destructive()")
	}

	res, err := p.Commit()
	if err != nil {
		t.Fatal(err)
	}

	dest, ok := res.BackedUp["AGENTS.md"]
	if !ok {
		t.Fatal("AGENTS.md overwritten with no backup recorded")
	}
	saved, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("backup unreadable: %v", err)
	}
	if string(saved) != handEdit {
		t.Fatalf("backup is not the pre-write bytes:\nwant %q\ngot  %q", handEdit, saved)
	}

	// And the overwrite did happen — otherwise this test passes for a Commit that
	// backs up and then declines to do its job.
	now, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if string(now) == handEdit {
		t.Fatal("Commit backed up but never overwrote")
	}
}

// TestApplyLeavesUnmanagedAlone: compile emits nothing for the path, so writing is a
// guess and deleting is a destructive guess.
func TestApplyLeavesUnmanagedAlone(t *testing.T) {
	root := t.TempDir()
	if _, err := planFor(t, root, "").Commit(); err != nil {
		t.Fatal(err)
	}

	// SOUL.md renders for this target; strip voice from the corpus and it becomes a
	// path the render map names but compile no longer fills.
	corpus := driftCorpus()
	trimmed := corpus[:0:0]
	for _, f := range corpus {
		if f.ID != "lead-with-outcome" {
			trimmed = append(trimmed, f)
		}
	}

	p, err := Plan(trimmed, klawOnM4(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range p.Files {
		if f.Path == "SOUL.md" && f.Action != ActionNone {
			t.Fatalf("unmanaged SOUL.md planned for %q", f.Action)
		}
	}

	if _, err := p.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "SOUL.md")); err != nil {
		t.Fatalf("apply removed an unmanaged file: %v", err)
	}
}

// TestApplyCleanFileIsNotDestructive: a fresh-machine plan creates files and touches
// nothing that exists, so it must not demand the same confirmation as an overwrite.
// Collapsing the two trains an operator to confirm without reading.
func TestApplyCleanFileIsNotDestructive(t *testing.T) {
	root := t.TempDir()
	if p := planFor(t, root, ""); p.Destructive() {
		t.Error("a plan that only creates missing files must not report Destructive()")
	}
	if _, err := planFor(t, root, "").Commit(); err != nil {
		t.Fatal(err)
	}
	if p := planFor(t, root, ""); p.Destructive() {
		t.Error("a clean tree must not report Destructive()")
	}
}

// TestApplyRejectsUnknownStatus: a new drift status must stop the build, not fall
// through to "do nothing" — that would be a compile step silently ceasing to manage a
// file, which looks exactly like success.
func TestApplyRejectsUnknownStatus(t *testing.T) {
	_, err := planFile(FileDrift{Path: "AGENTS.md", Status: "invented"}, Result{}, t.TempDir())
	if err == nil {
		t.Fatal("unknown drift status must error, not default to a no-op")
	}
	if !strings.Contains(err.Error(), "unhandled") {
		t.Errorf("error should name the cause, got: %v", err)
	}
}

// TestWriteAtomicLeavesNoPartial: a truncated rules file is worse than either the old
// or the new one — the agent loads it and obeys half a rulebook.
func TestWriteAtomicLeavesNoPartial(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	if err := writeAtomic(path, "new content"); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".soul-forge-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new content" {
		t.Errorf("got %q", got)
	}
}

// TestApplyReportsShrink pins the delta, which exists because of a real incident: a
// six-fragment corpus applied over the live workspace replaced a 6,632-byte AGENTS.md
// with 176 bytes. Correct for the corpus given, and catastrophic. --force stopped it,
// but --force fires identically for a one-line correction, so the operator needs the
// number to tell the two apart.
func TestApplyReportsShrink(t *testing.T) {
	root := t.TempDir()
	if _, err := planFor(t, root, "").Commit(); err != nil {
		t.Fatal(err)
	}

	big := "# AGENTS.md\n\n" + strings.Repeat("- A rule that exists only on disk.\n", 200)
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, f := range planFor(t, root, "").Files {
		if f.Path != "AGENTS.md" {
			continue
		}
		before, after := f.Delta()
		if before != len(big) {
			t.Errorf("before = %d, want the on-disk size %d", before, len(big))
		}
		if after >= before {
			t.Errorf("this apply shrinks the file; Delta reports %d -> %d", before, after)
		}
		return
	}
	t.Fatal("AGENTS.md absent from plan")
}
