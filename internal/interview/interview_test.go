package interview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cyperx84/soul-forge/internal/llm"
	"github.com/cyperx84/soul-forge/internal/profile"
)

type queueClient struct {
	responses []string
	errAt     int
	calls     int
}

func (q *queueClient) Chat(_ context.Context, _ []llm.Message, _ llm.ChatOpts) (string, error) {
	q.calls++
	if q.errAt > 0 && q.calls == q.errAt {
		return "", errors.New("boom")
	}
	if len(q.responses) == 0 {
		return "", errors.New("no responses left")
	}
	resp := q.responses[0]
	q.responses = q.responses[1:]
	return resp, nil
}

func TestExtractAndSaveAndSummary(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)

	iv := &Interview{}
	session := NewSession("openai", "gpt")
	session.AppendMessage("user", "My name is Chris")
	session.AppendMessage("assistant", "cool")
	ext := &Extractor{client: &fakeClient{resp: `{"identity":{"name":"Chris"},"environment":{"os":"macOS"}}`}, model: "m"}
	out := filepath.Join(dir, ".soul-forge", "profile.json")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	iv.extractAndSave(context.Background(), ext, session, out)
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(session.FieldsCaptured) == 0 {
		t.Fatalf("expected captured fields")
	}
	stderr := captureStderr(t, func() { iv.showSummary(out) })
	if !strings.Contains(stderr, "Interview complete") || !strings.Contains(stderr, "profile.json") {
		t.Fatalf("stderr=%s", stderr)
	}
}

func TestHelpers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.json")
	prof := &profile.Profile{}
	prof.Identity.Name = "Chris"
	if err := writeProfileJSON(path, prof); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Identity struct {
			Name string `json:"name"`
		} `json:"identity"`
	}
	data, _ := os.ReadFile(path)
	if err := jsonUnmarshal(data, &got); err != nil || got.Identity.Name != "Chris" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestRunFreshAndResume(t *testing.T) {
	dir := t.TempDir()
	oldwd, _ := os.Getwd()
	defer os.Chdir(oldwd)
	os.Chdir(dir)
	os.MkdirAll(".soul-forge", 0o755)

	runFlow := func(resume bool, stdin string, responses []string) string {
		t.Helper()
		iv := &Interview{
			cfg: Config{Provider: "openai", Model: "gpt", MaxTurns: 10, OutputPath: ".soul-forge/profile.json", Resume: resume},
			client: &queueClient{responses: responses},
		}
		oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
		defer func() { os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr }()
		inR, inW, _ := os.Pipe()
		outR, outW, _ := os.Pipe()
		errR, errW, _ := os.Pipe()
		os.Stdin, os.Stdout, os.Stderr = inR, outW, errW
		io.WriteString(inW, stdin)
		inW.Close()
		outCh := make(chan string, 2)
		go func() { var b bytes.Buffer; io.Copy(&b, outR); outCh <- b.String() }()
		go func() { var b bytes.Buffer; io.Copy(&b, errR); outCh <- b.String() }()
		if err := iv.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		outW.Close()
		errW.Close()
		return (<-outCh) + (<-outCh)
	}

	out := runFlow(false, "first answer\nbye\n", []string{"Hello there", `[COMPLETE] Thanks, done`, `{"identity":{"name":"Chris"}}`})
	if !strings.Contains(out, "Hello there") || !strings.Contains(out, "Thanks, done") {
		t.Fatalf("output=%s", out)
	}
	if _, err := os.Stat(filepath.Join(".soul-forge", "session.json")); err != nil {
		t.Fatal(err)
	}

	s := NewSession("openai", "gpt")
	s.AppendMessage("assistant", "resume me")
	s.FieldsCaptured = []string{"identity.name"}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	out = runFlow(true, "quit\n", []string{`{"identity":{"name":"Chris"}}`})
	if !strings.Contains(out, "Resuming session") {
		t.Fatalf("resume output=%s", out)
	}
}

func TestRunInitialMessageError(t *testing.T) {
	iv := &Interview{cfg: Config{Provider: "openai", Model: "gpt", MaxTurns: 1, OutputPath: filepath.Join(t.TempDir(), "profile.json")}, client: &queueClient{errAt: 1, responses: []string{"unused"}}}
	oldIn := os.Stdin
	defer func() { os.Stdin = oldIn }()
	r, w, _ := os.Pipe()
	os.Stdin = r
	w.Close()
	if err := iv.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "initial message failed") {
		t.Fatalf("got %v", err)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}
