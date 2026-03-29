package llm

import (
	"context"
	"fmt"
	"os"
)

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatOpts controls LLM generation parameters.
type ChatOpts struct {
	Temperature float64
	MaxTokens   int
}

// Client is the interface for LLM chat completions.
type Client interface {
	Chat(ctx context.Context, messages []Message, opts ChatOpts) (string, error)
}

// NewClient creates an LLM client for the given provider.
func NewClient(provider, model, apiKey, baseURL string) (Client, error) {
	switch provider {
	case "openai":
		key := firstNonEmpty(apiKey, os.Getenv("SOUL_FORGE_API_KEY"), os.Getenv("OPENAI_API_KEY"))
		url := firstNonEmpty(baseURL, "https://api.openai.com/v1")
		return newOpenAIClient(url, key, model), nil
	case "ollama":
		url := firstNonEmpty(baseURL, os.Getenv("OLLAMA_HOST"), "http://localhost:11434/v1")
		return newOpenAIClient(url, "", model), nil
	case "openrouter":
		key := firstNonEmpty(apiKey, os.Getenv("SOUL_FORGE_API_KEY"), os.Getenv("OPENROUTER_API_KEY"))
		url := firstNonEmpty(baseURL, "https://openrouter.ai/api/v1")
		return newOpenAIClient(url, key, model), nil
	case "anthropic":
		key := firstNonEmpty(apiKey, os.Getenv("SOUL_FORGE_API_KEY"), os.Getenv("ANTHROPIC_API_KEY"))
		url := firstNonEmpty(baseURL, "https://api.anthropic.com")
		return newAnthropicClient(url, key, model), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
