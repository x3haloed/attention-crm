package agent

import "testing"

func TestFunctionCall_NormalizedArguments(t *testing.T) {
	c := FunctionCall{
		Type:      "function_call",
		Name:      "ui.message",
		Arguments: []byte(`"{\"text\":\"hi\"}"`),
	}
	raw, err := c.NormalizedArguments()
	if err != nil {
		t.Fatalf("NormalizedArguments err: %v", err)
	}
	if string(raw) != `{"text":"hi"}` {
		t.Fatalf("raw = %s", string(raw))
	}
}

func TestFunctionCall_NormalizedArguments_Object(t *testing.T) {
	c := FunctionCall{
		Type:      "function_call",
		Name:      "ui.no_action",
		Arguments: []byte(`{"x":1}`),
	}
	raw, err := c.NormalizedArguments()
	if err != nil {
		t.Fatalf("NormalizedArguments err: %v", err)
	}
	if string(raw) != `{"x":1}` {
		t.Fatalf("raw = %s", string(raw))
	}
}
