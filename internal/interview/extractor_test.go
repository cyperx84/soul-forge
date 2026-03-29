package interview

import (
	"context"
	"errors"
	"testing"

	"github.com/cyperx84/soul-forge/internal/llm"
)

type fakeClient struct {
	resp string
	err  error
	seen []llm.Message
}

func (f *fakeClient) Chat(_ context.Context, messages []llm.Message, _ llm.ChatOpts) (string, error) {
	f.seen = messages
	return f.resp, f.err
}

func TestExtractorExtract(t *testing.T) {
	fc := &fakeClient{resp: "```json\n{\"identity\":{\"name\":\"Chris\"}}\n```"}
	e := NewExtractor(fc, "m")
	p, err := e.Extract(context.Background(), []msg{{Role: "user", Content: "I am Chris"}})
	if err != nil || p == nil || p.Identity.Name != "Chris" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
	if len(fc.seen) != 2 || fc.seen[0].Role != "system" {
		t.Fatalf("messages=%+v", fc.seen)
	}
}

func TestExtractorErrors(t *testing.T) {
	e := NewExtractor(&fakeClient{err: errors.New("boom")}, "m")
	if _, err := e.Extract(context.Background(), []msg{{Role: "user", Content: "hi"}}); err == nil {
		t.Fatal("expected error")
	}
	e = NewExtractor(&fakeClient{resp: "not-json"}, "m")
	p, err := e.Extract(context.Background(), []msg{{Role: "user", Content: "hi"}})
	if err != nil || p != nil {
		t.Fatalf("p=%+v err=%v", p, err)
	}
}
