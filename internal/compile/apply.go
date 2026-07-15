package compile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

// Apply is the first step in this pipeline that writes. Everything before it reads a
// corpus and proposes; this one puts bytes on disk over files an agent loads every
// session, and over files a human may have hand-edited since the last compile.
//
// Two properties carry that weight, and both are structural rather than advisory:
//
// Apply is defined over a Diff report, never over a Compile result directly. Diff
// already decided every path's status and — critically — refuses to report "missing"
// for a file it could not read. Re-deriving the write set from Compile would re-open
// exactly the hole Diff was built to close: a permission error reading AGENTS.md would
// look like absence, and absence means "write it".
//
// Planning and writing are different types. Plan returns an ApplyPlan; only Commit
// touches disk. There is no flag that makes Plan write and no default that makes
// Commit implicit — the same shape as ingest's Proposal/Confirm split, for the same
// reason. A tool whose default action overwrites hand-written files is a footgun no
// amount of documentation fixes.

// Actions an ApplyPlan can take on a path. Each maps from exactly one drift status;
// the mapping is the whole safety argument, so it lives in one switch (planFile).
const (
	// ActionCreate: compile emits it, disk does not have it. The fresh-machine case.
	ActionCreate = "create"

	// ActionOverwrite: disk has different content. The only destructive action, and
	// the only one that takes a backup. See ApplyPlan.Destructive.
	ActionOverwrite = "overwrite"

	// ActionScaffold: a lifecycle:instance path (MEMORY.md) that does not exist yet.
	// Compile owns its existence, never its contents — so this fires only on absence
	// and never becomes an overwrite. Invariant 3.
	ActionScaffold = "scaffold"

	// ActionNone: nothing to do. Clean files, populated skeletons, and unmanaged
	// leftovers all land here. Recorded rather than omitted so a plan is a complete
	// account of every path the tool considered, including the ones it declined.
	ActionNone = "none"
)

// FileAction is one planned operation on one path.
type FileAction struct {
	Path   string
	Action string

	// Status is the drift status this action was derived from. Carried so a plan can
	// be read back against a diff report without re-running either.
	Status string

	// Content is what Commit will write. Empty for ActionNone.
	Content string

	// Existing is the current on-disk content, present only for ActionOverwrite. It
	// is what gets backed up — held here so Commit backs up the exact bytes the plan
	// was computed against, not whatever the file says by the time Commit runs.
	Existing string

	// Reason states why this action and not another, in the words of the rule that
	// decided it. Displayed to the operator: a plan they cannot audit is a plan they
	// will approve without reading.
	Reason string
}

// Destructive reports whether this action can lose data that exists nowhere else.
func (a FileAction) Destructive() bool { return a.Action == ActionOverwrite }

// Delta returns the byte change an overwrite would make: current size, new size.
//
// This exists because of an observed failure, not a hypothetical one. Applying a
// six-fragment corpus over a real workspace replaced a 6,632-byte AGENTS.md with 176
// bytes — correct behaviour for the corpus it was given, and catastrophic. The --force
// gate did stop it, but --force fires identically for a benign re-apply and for that,
// so an operator who uses this weekly learns to pass --force without reading, and then
// the catastrophic case walks straight through a gate that is technically working.
//
// Reporting the delta is deliberately not a threshold. A cutoff ("refuse below -50%")
// would be another constant invented by its author — the same mistake as ingest's 0.15
// floor, which silently guillotined a real duplicate because the number felt right.
// The size change is a fact; what counts as too much shrinkage is the operator's call,
// and they can only make it if the number is in front of them.
func (a FileAction) Delta() (before, after int) {
	return len(a.Existing), len(a.Content)
}

// ApplyPlan is a complete account of what Commit would do. It is inert.
type ApplyPlan struct {
	Target string
	Root   string

	// BackupDir is where overwritten content is copied before being replaced. Empty
	// disables backups — permitted, because a caller applying into a clean git tree
	// already has a better backup than this tool can make, and a second one is noise.
	BackupDir string

	Files []FileAction
}

// Destructive reports whether the plan overwrites anything. This is the flag a caller
// should gate a confirmation prompt on: a plan that only creates missing files on a
// fresh machine is not the same risk as one that replaces a file someone edited.
func (p ApplyPlan) Destructive() bool {
	for _, f := range p.Files {
		if f.Destructive() {
			return true
		}
	}
	return false
}

// Changes returns only the actions that touch disk, in plan order.
func (p ApplyPlan) Changes() []FileAction {
	var out []FileAction
	for _, f := range p.Files {
		if f.Action != ActionNone {
			out = append(out, f)
		}
	}
	return out
}

// Plan computes what applying corpus to t under root would do. It never writes.
//
// The honest limitation, stated here because no code can fix it: an overwrite means
// disk differs from compile output, and Plan cannot tell whether that is a stale
// compile or a hand edit made five minutes ago. Both look identical from here. The
// backup is the entire mitigation, which is why BackupDir defaults on in the command
// layer and why Destructive() exists to force the question up to a human.
func Plan(corpus []fragment.Fragment, t Target, root, backupDir string) (ApplyPlan, error) {
	report, err := Diff(corpus, t, root)
	if err != nil {
		return ApplyPlan{}, err
	}

	res, err := Compile(corpus, t)
	if err != nil {
		return ApplyPlan{}, err
	}

	plan := ApplyPlan{Target: t.Name, Root: root, BackupDir: backupDir}
	for _, d := range report.Files {
		action, err := planFile(d, res, root)
		if err != nil {
			return ApplyPlan{}, err
		}
		plan.Files = append(plan.Files, action)
	}
	return plan, nil
}

// planFile maps one drift status to one action. Every status is handled explicitly and
// an unknown one is an error rather than a default — a new status silently falling
// through to "do nothing" would be a compile step quietly ceasing to manage a file.
func planFile(d FileDrift, res Result, root string) (FileAction, error) {
	a := FileAction{Path: d.Path, Status: d.Status}

	switch d.Status {
	case StatusClean:
		a.Action = ActionNone
		a.Reason = "disk matches compile output"

	case StatusMissing:
		content, ok := res.Files[d.Path]
		if !ok {
			// Diff reports missing for absent skeletons too; those have no compiled
			// content of their own and are scaffolded empty.
			a.Action = ActionScaffold
			a.Reason = "skeleton path absent: compile owns its existence"
			break
		}
		a.Action = ActionCreate
		a.Content = content
		a.Reason = "compile emits it, disk does not have it"

	case StatusDrifted:
		content, ok := res.Files[d.Path]
		if !ok {
			return FileAction{}, fmt.Errorf("apply %s: drifted with no compiled content", d.Path)
		}
		existing, exists, err := readFile(filepath.Join(root, d.Path))
		if err != nil {
			return FileAction{}, fmt.Errorf("apply %s: %w", d.Path, err)
		}
		if !exists {
			// Vanished between diff and plan. Refusing is right: the premise the plan
			// was computed on no longer holds, and guessing which way it moved is how
			// a tool overwrites something it never read.
			return FileAction{}, fmt.Errorf("apply %s: file vanished between diff and plan", d.Path)
		}
		a.Action = ActionOverwrite
		a.Content = content
		a.Existing = existing
		a.Reason = "disk content differs from compile output"

	case StatusSkeleton:
		// Present and divergent, which is correct: the runtime wrote it.
		a.Action = ActionNone
		a.Reason = "lifecycle:instance — runtime owns contents"

	case StatusUnmanaged:
		// Compile emitted nothing for it. Writing would be a guess and deleting would
		// be a destructive guess; the leftover is the operator's call, per Diff.
		a.Action = ActionNone
		a.Reason = "compile emits nothing for this path — operator's call"

	default:
		return FileAction{}, fmt.Errorf("apply %s: unhandled drift status %q", d.Path, d.Status)
	}

	return a, nil
}

// ApplyResult records what Commit actually did.
type ApplyResult struct {
	Written   []string
	Skipped   []string
	BackedUp  map[string]string // path -> backup file path
	BackupDir string
}

// Commit executes the plan. Backups happen before any write, so a failure partway
// through leaves every overwritten file recoverable rather than only the ones that
// happened to run first.
func (p ApplyPlan) Commit() (ApplyResult, error) {
	res := ApplyResult{BackedUp: map[string]string{}, BackupDir: p.BackupDir}

	if p.BackupDir != "" && p.Destructive() {
		if err := os.MkdirAll(p.BackupDir, 0o755); err != nil {
			return res, fmt.Errorf("backup dir: %w", err)
		}
		for _, f := range p.Files {
			if !f.Destructive() {
				continue
			}
			dest := filepath.Join(p.BackupDir, strings.ReplaceAll(f.Path, string(filepath.Separator), "_"))
			if err := os.WriteFile(dest, []byte(f.Existing), 0o644); err != nil {
				return res, fmt.Errorf("backup %s: %w", f.Path, err)
			}
			res.BackedUp[f.Path] = dest
		}
	}

	for _, f := range p.Files {
		if f.Action == ActionNone {
			res.Skipped = append(res.Skipped, f.Path)
			continue
		}
		full := filepath.Join(p.Root, f.Path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return res, fmt.Errorf("apply %s: %w", f.Path, err)
		}
		if err := writeAtomic(full, f.Content); err != nil {
			return res, fmt.Errorf("apply %s: %w", f.Path, err)
		}
		res.Written = append(res.Written, f.Path)
	}

	return res, nil
}

// writeAtomic writes via a temp file in the destination directory plus a rename.
//
// A crash midway through a plain write leaves a truncated AGENTS.md, and a truncated
// rules file is worse than either the old or the new one: the agent loads it and obeys
// half a rulebook. Rename is atomic on the same filesystem, so the file is only ever
// the old bytes or the new ones.
func writeAtomic(path, content string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".soul-forge-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed; cleans up on any failure below

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
