package inference

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type openAICompatChat struct {
	cfg  Config
	path string
	http *http.Client
}

func newOpenAICompatChat(cfg Config, path string) *openAICompatChat {
	return &openAICompatChat{
		cfg:  cfg,
		path: path,
		http: httpClientFor(cfg),
	}
}

type openAIChatReq struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	// Tools are supported by OpenAI chat-completions, but shapes vary between providers.
	Tools []ToolDef `json:"tools,omitempty"`
}

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content,omitempty"`
			ToolCalls []struct {
				Index    int    `json:"index,omitempty"`
				ID       string `json:"id,omitempty"`
				Type     string `json:"type,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function,omitempty"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *openAICompatChat) Stream(ctx context.Context, req Request, onEvent StreamHandler) (Result, error) {
	base := strings.TrimRight(strings.TrimSpace(c.cfg.BaseURL), "/")
	if base == "" {
		return Result{}, errors.New("base_url required")
	}
	model := strings.TrimSpace(c.cfg.Model)
	if model == "" {
		return Result{}, errors.New("model required")
	}

	payload := openAIChatReq{
		Model:    model,
		Messages: normalizeChatMessages(req.Messages),
		Stream:   true,
	}
	if len(req.Tools) > 0 {
		payload.Tools = req.Tools
	}

	b, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+c.path, bytes.NewReader(b))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(c.cfg.APIKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.cfg.APIKey))
	}
	for k, v := range c.cfg.Headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		httpReq.Header.Set(k, v)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return Result{}, fmt.Errorf("inference http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	_ = onEvent(StreamEvent{Type: "response.created"})

	var outText strings.Builder
	toolArgsByIndex := map[int]string{}
	toolNameByIndex := map[int]string{}

	err = ReadSSE(resp.Body, func(ev SSEEvent) error {
		data := bytes.TrimSpace(ev.Data)
		if len(data) == 0 {
			return nil
		}
		if bytes.Equal(data, []byte("[DONE]")) {
			return nil
		}

		var chunk chatChunk
		if err := json.Unmarshal(data, &chunk); err != nil {
			return nil
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			_ = onEvent(StreamEvent{Type: "error", Data: mustJSON(map[string]any{"message": chunk.Error.Message})})
			return nil
		}
		if len(chunk.Choices) == 0 {
			return nil
		}
		d := chunk.Choices[0].Delta
		if strings.TrimSpace(d.Content) != "" {
			outText.WriteString(d.Content)
			_ = onEvent(StreamEvent{Type: "response.output_text.delta", Data: mustJSON(map[string]any{"delta": d.Content})})
		}
		for _, tc := range d.ToolCalls {
			if strings.TrimSpace(tc.Function.Name) != "" {
				toolNameByIndex[tc.Index] = strings.TrimSpace(tc.Function.Name)
			}
			if tc.Function.Arguments != "" {
				toolArgsByIndex[tc.Index] = toolArgsByIndex[tc.Index] + tc.Function.Arguments
				name := toolNameByIndex[tc.Index]
				if name == "" {
					name = "tool"
				}
				_ = onEvent(StreamEvent{Type: "response.function_call_arguments.delta", Data: mustJSON(map[string]any{"name": name, "delta": tc.Function.Arguments})})
			}
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	_ = onEvent(StreamEvent{Type: "response.output_text.done", Data: mustJSON(map[string]any{"text": outText.String()})})

	var functionCalls []json.RawMessage
	for idx, args := range toolArgsByIndex {
		name := toolNameByIndex[idx]
		if name == "" {
			continue
		}
		functionCalls = append(functionCalls, mustJSON(map[string]any{
			"type":      "function_call",
			"name":      name,
			"arguments": args,
		}))
	}
	_ = onEvent(StreamEvent{Type: "response.completed"})

	return Result{
		OutputText:   outText.String(),
		FunctionCalls: functionCalls,
	}, nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func normalizeChatMessages(in []Message) []Message {
	out := make([]Message, 0, len(in))
	for _, m := range in {
		role := strings.TrimSpace(strings.ToLower(m.Role))
		if role == "" {
			role = "user"
		}
		// Chat-completions APIs generally support: system, user, assistant.
		// We treat developer as system for compatibility.
		if role == "developer" {
			role = "system"
		}
		out = append(out, Message{Role: role, Content: m.Content})
	}
	return out
}
