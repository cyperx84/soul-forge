package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfileJSONSchemaIsValid(t *testing.T) {
	s := ProfileJSONSchema()
	if strings.TrimSpace(s) == "" {
		t.Fatal("schema is empty")
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}

	props, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema has no properties object")
	}
	for _, section := range []string{"identity", "work_style", "environment"} {
		if _, ok := props[section]; !ok {
			t.Errorf("schema missing top-level section %q", section)
		}
	}
}
