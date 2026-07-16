package fragment

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProfileResolvesChain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "prime.json"), `{
		"name": "prime",
		"fragments": [{"id":"red-line","text":"Never exfiltrate private data.","host":"any","profile":"any","harness":"any","lifecycle":"authored","kind":"rule"}]
	}`)
	writeFile(t, filepath.Join(dir, "m1.json"), `{
		"name": "m1", "extends": "prime.json",
		"fragments": [{"id":"m1-fact","text":"Clean machine, GPG keys born here.","host":"m1","profile":"any","harness":"any","lifecycle":"authored","kind":"fact"}]
	}`)

	c, err := LoadProfile(filepath.Join(dir, "m1.json"))
	if err != nil {
		t.Fatal(err)
	}
	frags, overrides, err := c.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 0 {
		t.Fatalf("no overrides expected, got %v", overrides)
	}
	// Parent-first order: inherited doctrine before local additions.
	if len(frags) != 2 || frags[0].ID != "red-line" || frags[1].ID != "m1-fact" {
		t.Fatalf("want [red-line m1-fact], got %v", frags)
	}
}

func TestLoadProfileRecordsOverride(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "base.json"), `{
		"name": "base",
		"fragments": [{"id":"disk","text":"Disk is roomy.","host":"any","profile":"any","harness":"any","lifecycle":"authored","kind":"fact"}]
	}`)
	writeFile(t, filepath.Join(dir, "child.json"), `{
		"name": "child", "extends": "base.json",
		"fragments": [{"id":"disk","text":"Disk chronically tight.","host":"m4-mini","profile":"any","harness":"any","lifecycle":"authored","kind":"fact"}]
	}`)

	c, err := LoadProfile(filepath.Join(dir, "child.json"))
	if err != nil {
		t.Fatal(err)
	}
	frags, overrides, err := c.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 1 || overrides[0].ChildProfile != "child" {
		t.Fatalf("want one recorded override by child, got %v", overrides)
	}
	if len(frags) != 1 || frags[0].Text != "Disk chronically tight." {
		t.Fatalf("child definition must win in place, got %v", frags)
	}
}

func TestLoadProfileExtendsCycle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.json"), `{"name":"a","extends":"b.json","fragments":[]}`)
	writeFile(t, filepath.Join(dir, "b.json"), `{"name":"b","extends":"a.json","fragments":[]}`)
	if _, err := LoadProfile(filepath.Join(dir, "a.json")); err == nil {
		t.Fatal("extends cycle must error, not hang")
	}
}

func TestLoadProfileRequiresName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "anon.json"), `{"fragments":[]}`)
	if _, err := LoadProfile(filepath.Join(dir, "anon.json")); err == nil {
		t.Fatal("nameless profile must error: overrides are attributed by profile name")
	}
}

func TestIsProfileFileSniffs(t *testing.T) {
	if !IsProfileFile([]byte("  \n\t{\"name\":\"x\"}")) {
		t.Fatal("object with leading whitespace must sniff as profile")
	}
	if IsProfileFile([]byte("[{\"id\":\"a\"}]")) {
		t.Fatal("array must sniff as flat corpus")
	}
	if IsProfileFile([]byte("")) {
		t.Fatal("empty input is not a profile")
	}
}

func TestWriteProfileRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := WriteProfile(path, "derived", "", nil); err != nil {
		t.Fatal(err)
	}
	c, err := LoadProfile(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "derived" || c.Extends != nil || len(c.Fragments) != 0 {
		t.Fatalf("round trip lost data: %+v", c)
	}
}
