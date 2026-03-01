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

type lmStudio struct {
	cfg  Config
	http *http.Client
}

func newLMStudio(cfg Config) *lmStudio {
	return &lmStudio{cfg: cfg, http: httpClientFor(cfg)}
}

type lmStudioReq struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Tools    any       `json:"tools,omitempty"`
}

func (c *lmStudio) Stream(ctx context.Context, req Request, onEvent StreamHandler) (Result, error) {
	base := strings.TrimRight(strings.TrimSpace(c.cfg.BaseURL), "/")
	if base == "" {
		return Result{}, errors.New("base_url required")
	}
	model := strings.TrimSpace(c.cfg.Model)
	if model == "" {
		return Result{}, errors.New("model required")
	}

	payload := lmStudioReq{
		Model:    model,
		Messages: req.Messages,
		Stream:   true,
	}
	if len(req.Tools) > 0 {
		payload.Tools = req.Tools
	}
	b, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1/chat", bytes.NewReader(b))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
	toolName := ""
	var toolArgs strings.Builder

	err = ReadSSE(resp.Body, func(ev SSEEvent) error {
		if len(bytes.TrimSpace(ev.Data)) == 0 {
			return nil
		}

		switch strings.TrimSpace(ev.Event) {
		case "message.delta":
			var p struct {
				Content string `json:"content"`
			}
			_ = json.Unmarshal(ev.Data, &p)
			if strings.TrimSpace(p.Content) == "" {
				return nil
			}
			outText.WriteString(p.Content)
			_ = onEvent(StreamEvent{Type: "response.output_text.delta", Data: mustJSON(map[string]any{"delta": p.Content})})

		case "tool_call.start":
			var p struct {
				Tool string `json:"tool"`
			}
			_ = json.Unmarshal(ev.Data, &p)
			toolName = strings.TrimSpace(p.Tool)
			toolArgs.Reset()
			if toolName != "" {
				_ = onEvent(StreamEvent{Type: "response.output_item.added", Data: mustJSON(map[string]any{
					"item": map[string]any{"type": "function_call", "name": toolName},
				})})
			}

		case "tool_call.arguments":
			var p struct {
				Tool      string          `json:"tool"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(ev.Data, &p)
			if toolName == "" {
				toolName = strings.TrimSpace(p.Tool)
			}
			if len(bytes.TrimSpace(p.Arguments)) == 0 {
				return nil
			}
			toolArgs.Write(bytes.TrimSpace(p.Arguments))
			_ = onEvent(StreamEvent{Type: "response.function_call_arguments.delta", Data: mustJSON(map[string]any{
				"name":  toolName,
				"delta": string(bytes.TrimSpace(p.Arguments)),
			})})
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	_ = onEvent(StreamEvent{Type: "response.output_text.done", Data: mustJSON(map[string]any{"text": outText.String()})})

	var functionCalls []json.RawMessage
	if strings.TrimSpace(toolName) != "" && strings.TrimSpace(toolArgs.String()) != "" {
		functionCalls = append(functionCalls, mustJSON(map[string]any{
			"type":      "function_call",
			"name":      toolName,
			"arguments": toolArgs.String(),
		}))
	}
	_ = onEvent(StreamEvent{Type: "response.completed"})

	return Result{OutputText: outText.String(), FunctionCalls: functionCalls}, nil
}

