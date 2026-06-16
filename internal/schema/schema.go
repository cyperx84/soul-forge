// Package schema exposes the machine-readable JSON Schema for a soul-forge profile,
// so any agent harness can drive an onboarding interview and emit a valid profile.json
// without soul-forge ever calling an LLM itself.
package schema

import _ "embed"

//go:embed profile.schema.json
var profileSchema string

// ProfileJSONSchema returns the JSON Schema (draft-07) describing profile.json.
func ProfileJSONSchema() string {
	return profileSchema
}
