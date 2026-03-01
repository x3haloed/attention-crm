package inference

import "encoding/json"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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

// StreamEvent is an OpenAI-style event envelope we can emit over SSE.
type StreamEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}
