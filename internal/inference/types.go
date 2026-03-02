package inference

import "encoding/json"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content,omitempty"`

	// tool_call_id is used for role="tool" messages in chat-completions.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// tool_calls is used for role="assistant" messages that request tools.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ToolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters,omitempty"`
		Strict      bool            `json:"strict,omitempty"`
	} `json:"function"`
}

type ToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function,omitempty"`
}

// StreamEvent is an OpenAI-style event envelope we can emit over SSE.
type StreamEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}
