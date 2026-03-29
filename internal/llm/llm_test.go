package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestFirstNonEmptyAndNewClient(t *testing.T) {
	if firstNonEmpty("", "a", "b") != "a" {
		t.Fatal("firstNonEmpty failed")
	}
	os.Setenv("OPENAI_API_KEY", "envkey")
	t.Cleanup(func() {
		os.Unsetenv("OPENAI_API_KEY")
		os.Unsetenv("ANTHROPIC_API_KEY")
	})
	c, err := NewClient("openai", "gpt", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*openAIClient); !ok {
		t.Fatalf("wrong type %T", c)
	}
	c, err = NewClient("anthropic", "claude", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.(*anthropicClient); !ok {
		t.Fatalf("wrong type %T", c)
	}
	if _, err := NewClient("nope", "m", "", ""); err == nil {
		t.Fatal("expected provider error")
	}
}

func TestOpenAIChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer key" {
			t.Fatalf("auth=%q", got)
		}
		var req openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.Model != "gpt" || len(req.Messages) != 1 || req.MaxTokens != 99 {
			t.Fatalf("req=%+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"hello"}}]}`))
	}))
	defer srv.Close()
	c := newOpenAIClient(srv.URL, "key", "gpt")
	got, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, ChatOpts{Temperature: 0.2, MaxTokens: 99})
	if err != nil || got != "hello" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestOpenAIErrors(t *testing.T) {
	tests := []struct {
		body   string
		status int
		want   string
	}{
		{"bad", 500, "API error 500"},
		{`{"choices":[]}`, 200, "no response from API"},
		{`{"error":{"message":"nope"}}`, 200, "API error: nope"},
		{`not-json`, 200, "parse response"},
	}
	for _, tt := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tt.status)
			w.Write([]byte(tt.body))
		}))
		c := newOpenAIClient(srv.URL, "", "gpt")
		_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, ChatOpts{})
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("err=%v want %q", err, tt.want)
		}
	}
}

func TestAnthropicChatAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "key" || r.Header.Get("anthropic-version") == "" {
			t.Fatalf("headers missing")
		}
		var req anthropicRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.System != "sys" || len(req.Messages) != 1 || req.MaxTokens != 4096 {
			t.Fatalf("req=%+v", req)
		}
		w.Write([]byte(`{"content":[{"text":"hello"}]}`))
	}))
	defer srv.Close()
	c := newAnthropicClient(srv.URL, "key", "claude")
	got, err := c.Chat(context.Background(), []Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "hi"}}, ChatOpts{})
	if err != nil || got != "hello" {
		t.Fatalf("got=%q err=%v", got, err)
	}

	for _, tt := range []struct {
		body   string
		status int
		want   string
	}{
		{"bad", 500, "API error 500"},
		{`{"content":[]}`, 200, "no response from API"},
		{`{"error":{"message":"bad"}}`, 200, "API error: bad"},
		{`no`, 200, "parse response"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tt.status)
			w.Write([]byte(tt.body))
		}))
		c := newAnthropicClient(srv.URL, "key", "claude")
		_, err := c.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, ChatOpts{MaxTokens: 1})
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("err=%v want %q", err, tt.want)
		}
	}
}
