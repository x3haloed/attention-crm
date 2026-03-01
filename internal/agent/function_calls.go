package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

// FunctionCall mirrors the essential fields the OpenAI Responses API returns for a tool call item
// of type "function_call".
type FunctionCall struct {
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	CallID    string          `json:"call_id,omitempty"`
}

func (c FunctionCall) NormalizedArguments() (json.RawMessage, error) {
	raw := bytes.TrimSpace(c.Arguments)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return []byte("{}"), nil
	}

	// Responses API commonly encodes arguments as a JSON string.
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return []byte("{}"), nil
		}
		raw = []byte(s)
	}

	switch raw[0] {
	case '{', '[':
		// Accept objects/arrays.
		var tmp any
		if err := json.Unmarshal(raw, &tmp); err != nil {
			return nil, err
		}
		return json.RawMessage(raw), nil
	default:
		return nil, errors.New("invalid function_call arguments")
	}
}

