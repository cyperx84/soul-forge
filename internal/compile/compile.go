// Package compile turns a scope-tagged fragment corpus into a harness's guidance
// files. It is the heart of v2 and it is not written yet — deliberately.
//
// Per V2-SPEC.md's build order, the role-bleed invariant test lands before the
// compiler that must satisfy it, and ships red. That test is the regression for the
// break that killed the hand-built ownership matrix: compiling one agent leaked
// another agent's identity, because a file can only be one bucket and a line was
// filed by file. Writing the test first means every later feature is judged against
// it rather than negotiated with it.
//
// Result and Target are real; Compile is the stub. The API surface exists so the
// invariant test asserts real behavior and fails on the answer, not on a build error.
package compile

import (
	"errors"

	"github.com/cyperx84/soul-forge/internal/fragment"
)

// ErrNotImplemented is returned by Compile until the compiler exists. Its presence
// is what makes the role-bleed test red rather than absent.
var ErrNotImplemented = errors.New("compile: not implemented — the role-bleed invariant test ships before the compiler (see V2-SPEC.md build order)")

// Target is one compile destination: a concrete point in scope space plus the
// harness layout to render into.
type Target struct {
	// Name identifies the target ("openclaw-hub", "claude-global").
	Name string

	// Selector is the scope point this target compiles for.
	Selector fragment.Selector
}

// Result is a compiled target: the file set it produced, keyed by output path
// relative to the target's root.
type Result struct {
	// Files maps output path to rendered content ("AGENTS.md" -> "# AGENTS.md...").
	Files map[string]string

	// Selected is every fragment that survived selection, in corpus order. It is
	// carried so invariants and audit can assert over fragments rather than parse
	// rendered strings back into meaning.
	Selected []fragment.Fragment
}

// Compile renders corpus into t's file set, enforcing the compile-time invariants.
//
// Not implemented. See package doc.
func Compile(corpus []fragment.Fragment, t Target) (Result, error) {
	return Result{}, ErrNotImplemented
}
