package fragment

import (
	"fmt"
	"sort"
)

// Corpus is a fragment store plus its inheritance chain.
//
// Profiles inherit: prime (the clean machine where GPG keys are born) holds every
// host:any, profile:any fragment; each machine's profile adds host:<id> fragments;
// each agent adds profile:<id> fragments. A child may add or override, but never
// silently drops a parent fragment — a silent drop is how a red line disappears from
// a downstream box without anyone noticing.
type Corpus struct {
	// Name identifies this profile ("prime", "m4-mini", "klaw").
	Name string

	// Extends is the parent profile this one derives from. Empty for prime.
	Extends *Corpus

	// Fragments are this profile's own, before inheritance is resolved.
	Fragments []Fragment
}

// Override is a child's deliberate replacement of an inherited fragment: same ID,
// different content. Resolve records every one so `audit` can show what a downstream
// box changed about its parent, rather than leaving it to be discovered by surprise.
type Override struct {
	ID     string
	Parent Fragment
	Child  Fragment
	// ChildProfile names the profile that performed the override.
	ChildProfile string
}

// Resolve flattens the inheritance chain into one corpus.
//
// Order is parent-first, so a reader of compiled output sees inherited doctrine
// before local additions. A child fragment with the same ID as a parent's replaces it
// in place — keeping the parent's position, because an override is a redefinition,
// not a re-prioritisation. Resolution is deterministic: same chain in, same slice out.
func (c *Corpus) Resolve() ([]Fragment, []Override, error) {
	if c == nil {
		return nil, nil, nil
	}
	chain, err := c.chain()
	if err != nil {
		return nil, nil, err
	}

	var out []Fragment
	index := map[string]int{} // fragment ID -> position in out
	var overrides []Override

	for _, profile := range chain {
		for _, f := range profile.Fragments {
			if err := f.Validate(); err != nil {
				return nil, nil, fmt.Errorf("profile %q: %w", profile.Name, err)
			}
			if pos, seen := index[f.ID]; seen {
				overrides = append(overrides, Override{
					ID: f.ID, Parent: out[pos], Child: f, ChildProfile: profile.Name,
				})
				out[pos] = f // override in place: keep the parent's position
				continue
			}
			index[f.ID] = len(out)
			out = append(out, f)
		}
	}
	return out, overrides, nil
}

// chain returns the inheritance chain root-first (prime, then machine, then agent).
func (c *Corpus) chain() ([]*Corpus, error) {
	var chain []*Corpus
	seen := map[*Corpus]bool{}
	seenName := map[string]bool{}

	for p := c; p != nil; p = p.Extends {
		// A cycle would otherwise hang the compiler. Cheap to check, and an
		// extends-loop is an easy mistake to make when cloning profiles by hand.
		if seen[p] {
			return nil, fmt.Errorf("corpus %q: extends cycle", p.Name)
		}
		if seenName[p.Name] {
			return nil, fmt.Errorf("corpus %q: extends cycle (duplicate profile name in chain)", p.Name)
		}
		seen[p] = true
		seenName[p.Name] = true
		chain = append(chain, p)
	}

	// Reverse: walked child-to-root, want root-to-child.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain, nil
}

// DuplicateIDs reports fragment IDs defined more than once within a single profile.
// Cross-profile reuse of an ID is an override and legitimate; reuse inside one
// profile is a mistake, because the two definitions have no defined precedence.
func DuplicateIDs(fragments []Fragment) []string {
	count := map[string]int{}
	for _, f := range fragments {
		count[f.ID]++
	}
	var dupes []string
	for id, n := range count {
		if n > 1 {
			dupes = append(dupes, id)
		}
	}
	sort.Strings(dupes)
	return dupes
}
