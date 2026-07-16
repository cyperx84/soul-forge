package fragment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// profileFile is the on-disk shape of a profile: a named fragment set plus an
// optional parent it extends. This is the provisioning primitive's file format —
// prime holds the universal fragments, a machine profile extends prime, an agent
// profile extends its machine. `clone` writes these; every command that loads a
// corpus accepts them.
type profileFile struct {
	Name string `json:"name"`
	// Extends is a path to the parent profile, relative to this file's directory.
	// Relative on purpose: a profile tree must survive being moved to another box,
	// which is the whole reason it exists.
	Extends   string     `json:"extends,omitempty"`
	Fragments []Fragment `json:"fragments"`
}

// LoadProfile reads a profile file and its extends chain from disk, returning the
// child Corpus with parents linked. Callers get the flattened fragment set (and the
// overrides a child performed) from Resolve.
func LoadProfile(path string) (*Corpus, error) {
	return loadProfile(path, map[string]bool{})
}

func loadProfile(path string, visiting map[string]bool) (*Corpus, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("profile %s: %w", path, err)
	}
	// Cycle detection by path, before Resolve's by-name check ever runs — a cycle
	// here would loop file loading itself, not just resolution.
	if visiting[abs] {
		return nil, fmt.Errorf("profile %s: extends cycle", path)
	}
	visiting[abs] = true

	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("profile: %w", err)
	}
	var pf profileFile
	if err := json.Unmarshal(b, &pf); err != nil {
		return nil, fmt.Errorf("profile %s: %w", path, err)
	}
	if pf.Name == "" {
		return nil, fmt.Errorf("profile %s: name is required", path)
	}

	c := &Corpus{Name: pf.Name, Fragments: pf.Fragments}
	if pf.Extends != "" {
		parentPath := pf.Extends
		if !filepath.IsAbs(parentPath) {
			parentPath = filepath.Join(filepath.Dir(abs), parentPath)
		}
		parent, err := loadProfile(parentPath, visiting)
		if err != nil {
			return nil, fmt.Errorf("profile %s: extends: %w", path, err)
		}
		c.Extends = parent
	}
	return c, nil
}

// IsProfileFile reports whether raw JSON is a profile object rather than a bare
// fragment array. Sniffed on the first non-space byte: a profile is an object, a
// flat corpus is an array. Both stay valid inputs everywhere — onboard and review
// emit arrays, clone emits profiles, and a loader that accepted only one would
// orphan the other's output.
func IsProfileFile(raw []byte) bool {
	for _, ch := range raw {
		switch ch {
		case ' ', '\t', '\r', '\n':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}

// WriteProfile serialises a profile to disk. Used by clone; kept beside the loader
// so the two formats cannot drift apart.
func WriteProfile(path, name, extends string, frags []Fragment) error {
	if frags == nil {
		frags = []Fragment{} // render as [], not null: an empty set is not an absent one
	}
	pf := profileFile{Name: name, Extends: extends, Fragments: frags}
	b, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}
