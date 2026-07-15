package compile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

// Drift statuses. A path gets exactly one.
const (
	// StatusClean: disk matches compile output byte for byte.
	StatusClean = "clean"

	// StatusDrifted: the file exists and its content differs from compile output.
	// This is the case the detector exists for — a hand edit that will be silently
	// reverted by the next apply, or a compile change nobody landed.
	StatusDrifted = "drifted"

	// StatusMissing: compile emits this path and disk does not have it. Drift, not
	// an error: it is what a fresh machine looks like before its first apply.
	StatusMissing = "missing"

	// StatusSkeleton: a lifecycle:instance path (MEMORY.md). Present on disk, content
	// differs from the scaffold, and that is correct — the runtime wrote it. Never
	// drift. Invariant 3: runtime memory is not the compiler's business.
	StatusSkeleton = "skeleton"

	// StatusUnmanaged: a path this target's render map names, present on disk, that
	// compile emitted nothing for. Reported and not drift: the fragment that used to
	// render there may simply have been retired, and the leftover file is a fact the
	// operator should decide about rather than one the tool should act on.
	StatusUnmanaged = "unmanaged"
)

// FileDrift is the comparison of one output path against disk.
type FileDrift struct {
	Path   string
	Status string

	// OnlyCompiled and OnlyLive are the lines each side has and the other does not.
	// They explain a drift; they do not define it — Status is decided on raw bytes,
	// because a reordering is a real change and a line-set comparison would call it
	// clean. Both empty on a drifted file means the lines are identical and the order
	// is not; Reordered says so.
	OnlyCompiled []string
	OnlyLive     []string
	Reordered    bool
}

// Report is a target's full drift comparison.
type Report struct {
	Target string
	Root   string
	Files  []FileDrift
}

// HasDrift reports whether anything a compile owns has moved out from under it. This
// is the cron/CI signal: exit non-zero on true.
//
// Skeleton and unmanaged paths are deliberately excluded. A drift detector that fires
// on things nobody should act on trains its reader to ignore it, and an ignored alarm
// is worse than no alarm — it costs the same and buys a false sense of coverage.
func (r Report) HasDrift() bool {
	for _, f := range r.Files {
		if f.Status == StatusDrifted || f.Status == StatusMissing {
			return true
		}
	}
	return false
}

// Diff compiles corpus for t and compares the result against the files under root.
//
// It never writes. Every failure mode a caller can act on is a status on a path, not
// an error; an error from Diff means the comparison itself could not be made (the
// corpus does not compile, a file exists but cannot be read).
func Diff(corpus []fragment.Fragment, t Target, root string) (Report, error) {
	res, err := Compile(corpus, t)
	if err != nil {
		return Report{}, err
	}

	rm, err := t.renderMap()
	if err != nil {
		return Report{}, err
	}

	report := Report{Target: t.Name, Root: root}

	// Every path either side knows about: compiled output plus render-map paths that
	// compile emitted nothing for. The second set is why a retired fragment does not
	// leave an orphaned file unmentioned.
	paths := map[string]bool{}
	for p := range res.Files {
		paths[p] = true
	}
	for _, p := range rm.Order {
		paths[p] = true
	}

	for _, path := range sortedSet(paths) {
		compiled, isCompiled := res.Files[path]
		live, exists, err := readFile(filepath.Join(root, path))
		if err != nil {
			return Report{}, fmt.Errorf("diff %s: %s: %w", t.Name, path, err)
		}

		switch {
		case contains(rm.Skeleton, path):
			// Compile owns this file's existence, never its contents.
			if !exists {
				report.Files = append(report.Files, FileDrift{Path: path, Status: StatusMissing})
				continue
			}
			report.Files = append(report.Files, FileDrift{Path: path, Status: StatusSkeleton})

		case !isCompiled:
			if !exists {
				continue // neither side has it: nothing to say
			}
			report.Files = append(report.Files, FileDrift{Path: path, Status: StatusUnmanaged})

		case !exists:
			report.Files = append(report.Files, FileDrift{Path: path, Status: StatusMissing,
				OnlyCompiled: lines(compiled)})

		case live == compiled:
			report.Files = append(report.Files, FileDrift{Path: path, Status: StatusClean})

		default:
			d := FileDrift{Path: path, Status: StatusDrifted}
			d.OnlyCompiled, d.OnlyLive = lineSetDiff(compiled, live)
			d.Reordered = len(d.OnlyCompiled) == 0 && len(d.OnlyLive) == 0
			report.Files = append(report.Files, d)
		}
	}

	return report, nil
}

// readFile returns a file's content and whether it exists. A missing file is a status,
// not an error; anything else (a permission failure, a directory where a file should
// be) is an error, because reporting "missing" for a file we simply could not read
// would be the detector lying in the one direction that matters.
func readFile(path string) (string, bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

// lineSetDiff reports the lines each side holds that the other does not, counting
// duplicates. Explanatory only — Status is already decided on bytes.
func lineSetDiff(compiled, live string) (onlyCompiled, onlyLive []string) {
	cCount := countLines(compiled)
	lCount := countLines(live)
	return surplus(lines(compiled), lCount), surplus(lines(live), cCount)
}

// surplus returns the entries of side occurring more often in side than in theirs,
// preserving side's order and reporting each excess occurrence once.
func surplus(side []string, theirs map[string]int) []string {
	var out []string
	seen := map[string]int{}
	for _, line := range side {
		seen[line]++
		if seen[line] > theirs[line] {
			out = append(out, line)
		}
	}
	return out
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func countLines(s string) map[string]int {
	m := map[string]int{}
	for _, l := range lines(s) {
		m[l]++
	}
	return m
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
