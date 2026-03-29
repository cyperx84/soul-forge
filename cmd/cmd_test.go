package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunQuestions(t *testing.T) {
	old := questionsFormat
	questionsFormat = "md"
	defer func() { questionsFormat = old }()
	out := captureStdout(t, func() {
		if err := runQuestions(&cobra.Command{}, nil); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "# Part 1") {
		t.Fatalf("stdout=%s", out)
	}
	questionsFormat = "json"
	out = captureStdout(t, func() {
		if err := runQuestions(&cobra.Command{}, nil); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, `"part": 1`) {
		t.Fatalf("stdout=%s", out)
	}
	questionsFormat = "wat"
	if err := runQuestions(&cobra.Command{}, nil); err == nil {
		t.Fatal("expected format error")
	}
}

func TestRunInitImportGenerateAudit(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)

	noAnimation, initForce = true, false
	if err := runInit(&cobra.Command{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := runInit(&cobra.Command{}, nil); err == nil {
		t.Fatal("expected existing file error")
	}
	initForce = true
	if err := runInit(&cobra.Command{}, nil); err != nil {
		t.Fatal(err)
	}

	profileJSON := `{"identity":{"name":"Chris"},"work_style":{"preferences":["speed"]},"environment":{"os":"macOS"}}`
	inputProfile := filepath.Join(dir, "profile.json")
	os.WriteFile(inputProfile, []byte(profileJSON), 0o644)

	importMerge = false
	if err := runImport(&cobra.Command{}, []string{inputProfile}); err != nil {
		t.Fatal(err)
	}

	generateAgent, generateAll, generateDryRun, generateNoAnim = "", true, false, true
	if err := runGenerate(&cobra.Command{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "agents", "assistant", "USER.md")); err != nil {
		t.Fatal(err)
	}

	auditAgent, auditAll = "", true
	if err := runAudit(&cobra.Command{}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunGenerateAndAuditErrors(t *testing.T) {
	generateAgent, generateAll = "one", true
	if err := runGenerate(&cobra.Command{}, nil); err == nil {
		t.Fatal("expected mutual exclusion error")
	}
	generateAgent, generateAll = "", false
	if err := runGenerate(&cobra.Command{}, nil); err == nil {
		t.Fatal("expected missing selector error")
	}
	auditAgent, auditAll = "one", true
	if err := runAudit(&cobra.Command{}, nil); err == nil {
		t.Fatal("expected mutual exclusion error")
	}
	auditAgent, auditAll = "", false
	if err := runAudit(&cobra.Command{}, nil); err == nil {
		t.Fatal("expected missing selector error")
	}
}

func TestRunInterviewErrors(t *testing.T) {
	interviewProvider = "unsupported"
	if err := runInterview(&cobra.Command{}, nil); err == nil {
		t.Fatal("expected error")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}
