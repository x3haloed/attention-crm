package inference

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type ProviderKind string

const (
	ProviderOpenAI    ProviderKind = "openai"
	ProviderOpenRouter ProviderKind = "openrouter"
	ProviderLMStudio  ProviderKind = "lmstudio"
)

type Config struct {
	Provider ProviderKind
	BaseURL  string
	Model    string
	APIKey   string
	Headers  map[string]string
	Timeout  time.Duration
}

type Request struct {
	Messages []Message `json:"messages"`
	Tools    []ToolDef `json:"tools,omitempty"`
}

type Result struct {
	OutputText   string            `json:"output_text"`
	FunctionCalls []json.RawMessage `json:"function_calls,omitempty"`
}

type StreamHandler func(StreamEvent) error

type Client interface {
	Stream(ctx context.Context, req Request, onEvent StreamHandler) (Result, error)
}

func New(cfg Config) (Client, error) {
	switch strings.TrimSpace(string(cfg.Provider)) {
	case string(ProviderOpenAI):
		return newOpenAICompatChat(cfg, "/v1/chat/completions"), nil
	case string(ProviderOpenRouter):
		return newOpenAICompatChat(cfg, "/api/v1/chat/completions"), nil
	case string(ProviderLMStudio):
		// LM Studio exposes an OpenAI-compatible API at /v1/chat/completions.
		return newOpenAICompatChat(cfg, "/v1/chat/completions"), nil
	default:
		return nil, errors.New("unknown provider")
	}
}

func httpClientFor(cfg Config) *http.Client {
	to := cfg.Timeout
	if to <= 0 {
		to = 60 * time.Second
	}
	return &http.Client{Timeout: to}
}
