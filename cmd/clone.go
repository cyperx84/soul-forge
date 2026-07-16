package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cyperx84/soul-forge/internal/fragment"
	"github.com/spf13/cobra"
)

// clone is the provisioning primitive: prime holds the universal fragments, each
// machine extends prime, each agent extends its machine. A clone starts as an empty
// child — inheritance does the carrying, so a new box begins with every parent rule
// and zero copied text. Copying would be the hand-sync this tool exists to kill.

var (
	cloneAs     string
	cloneOut    string
	cloneSets   []string
	cloneRetags []string
	cloneForce  bool
)

var cloneCmd = &cobra.Command{
	Use:   "clone <base-profile.json>",
	Short: "Derive a new profile (machine or agent) extending a base profile",
	Long: `Writes a child profile that extends the base. The child starts empty:
every base fragment is inherited, none are copied, so a fix in the base reaches
every clone on its next compile.

--set <fragment-id>=<new text> overrides an inherited fragment in the child —
same scope tags, new text. --retag <fragment-id>=<axis>:<value> overrides a
scope tag (axis: host, profile, or harness) — needed when an override's new
text is machine-specific and the inherited host:any would misroute it. Both
are recorded by resolve and reported by audit, never silent. Unknown ids are
an error: overriding a fragment that does not exist is a typo, not a
customisation.`,
	Args: cobra.ExactArgs(1),
	RunE: runClone,
}

func init() {
	rootCmd.AddCommand(cloneCmd)
	cloneCmd.Flags().StringVar(&cloneAs, "as", "", "name for the derived profile (required)")
	cloneCmd.Flags().StringVar(&cloneOut, "out", "", "output path (default <name>.json beside the base)")
	cloneCmd.Flags().StringArrayVar(&cloneSets, "set", nil, "override an inherited fragment: <fragment-id>=<new text> (repeatable)")
	cloneCmd.Flags().StringArrayVar(&cloneRetags, "retag", nil, "override a scope tag: <fragment-id>=<axis>:<value>, axis one of host, profile, harness (repeatable)")
	cloneCmd.Flags().BoolVar(&cloneForce, "force", false, "overwrite an existing output file")
	_ = cloneCmd.MarkFlagRequired("as")
}

func runClone(cmd *cobra.Command, args []string) error {
	basePath := args[0]

	// Load and resolve the base first: a clone of a broken chain is a broken chain
	// with one more link, and the error should land now, not at first compile.
	base, err := fragment.LoadProfile(basePath)
	if err != nil {
		return err
	}
	baseFrags, _, err := base.Resolve()
	if err != nil {
		return fmt.Errorf("base %s: %w", basePath, err)
	}
	if cloneAs == base.Name {
		return fmt.Errorf("--as %q collides with the base profile's name: overrides are attributed by name, and a duplicate makes the chain report a cycle", cloneAs)
	}

	overrides, err := buildOverrides(cloneSets, cloneRetags, baseFrags)
	if err != nil {
		return err
	}

	outPath := cloneOut
	if outPath == "" {
		outPath = filepath.Join(filepath.Dir(basePath), cloneAs+".json")
	}
	if _, err := os.Stat(outPath); err == nil && !cloneForce {
		return fmt.Errorf("%s exists; refusing to overwrite without --force", outPath)
	}

	// The child references its base relatively, so the profile tree survives being
	// moved to the box it is provisioning.
	extends, err := relativeTo(outPath, basePath)
	if err != nil {
		return err
	}

	if err := fragment.WriteProfile(outPath, cloneAs, extends, overrides); err != nil {
		return fmt.Errorf("clone: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "%s: extends %s (%d inherited fragments, %d overridden)\n",
		outPath, base.Name, len(baseFrags), len(overrides))
	return nil
}

// buildOverrides turns --set id=text and --retag id=axis:value pairs into child
// fragments: the parent's definition with replaced text and/or tags. Same ID is
// what makes Resolve treat it as an override rather than an addition. A retag
// without a set is legitimate — narrowing an inherited fragment's scope for this
// chain without touching its text.
func buildOverrides(sets, retags []string, baseFrags []fragment.Fragment) ([]fragment.Fragment, error) {
	byID := map[string]fragment.Fragment{}
	for _, f := range baseFrags {
		byID[f.ID] = f
	}

	// Accumulate per-id so one fragment can take both a --set and --retag, or
	// several retags, without emitting duplicate child IDs (which compile rejects:
	// two definitions have no defined precedence).
	children := map[string]fragment.Fragment{}
	var order []string
	childOf := func(id string) (fragment.Fragment, bool) {
		if c, ok := children[id]; ok {
			return c, true
		}
		parent, ok := byID[id]
		if !ok {
			return fragment.Fragment{}, false
		}
		order = append(order, id)
		return parent, true
	}

	for _, s := range sets {
		id, text, found := strings.Cut(s, "=")
		if !found || strings.TrimSpace(id) == "" || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("--set %q: want <fragment-id>=<new text>", s)
		}
		child, ok := childOf(id)
		if !ok {
			return nil, fmt.Errorf("--set %s: no fragment with that id in the base chain", id)
		}
		child.Text = text
		children[id] = child
	}

	for _, r := range retags {
		id, tag, found := strings.Cut(r, "=")
		if !found {
			return nil, fmt.Errorf("--retag %q: want <fragment-id>=<axis>:<value>", r)
		}
		axis, value, found := strings.Cut(tag, ":")
		if !found || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("--retag %q: want <fragment-id>=<axis>:<value>", r)
		}
		child, ok := childOf(id)
		if !ok {
			return nil, fmt.Errorf("--retag %s: no fragment with that id in the base chain", id)
		}
		switch axis {
		case "host":
			child.Host = value
		case "profile":
			child.Profile = value
		case "harness":
			child.Harness = value
		default:
			// Lifecycle and kind are deliberately not retaggable: changing them makes
			// a different fragment, not a rescoped one — author it instead.
			return nil, fmt.Errorf("--retag %s: axis %q is not retaggable; want host, profile, or harness", id, axis)
		}
		if err := child.Validate(); err != nil {
			return nil, fmt.Errorf("--retag %s: %w", id, err)
		}
		children[id] = child
	}

	var out []fragment.Fragment
	for _, id := range order {
		out = append(out, children[id])
	}
	return out, nil
}

// relativeTo computes the extends path from the child file's directory to the base.
// Falls back to absolute only when no relative path exists (different volumes).
func relativeTo(childPath, basePath string) (string, error) {
	childAbs, err := filepath.Abs(childPath)
	if err != nil {
		return "", err
	}
	baseAbs, err := filepath.Abs(basePath)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(filepath.Dir(childAbs), baseAbs)
	if err != nil {
		return baseAbs, nil
	}
	return rel, nil
}
