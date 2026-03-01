package app

import (
	"attention-crm/internal/agent"
	"attention-crm/internal/control"
	"attention-crm/internal/inference"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) kickShadowRunAsync(tenant control.Tenant, sess session, r *http.Request) {
	if r == nil {
		return
	}
	// Avoid recursion: do not trigger from agent endpoints.
	if strings.Contains(r.URL.Path, "/agent/") {
		return
	}

	key := shadowSessionKey(r, sess, tenant)
	s.shadowMu.Lock()
	if s.shadowRunning[key] {
		s.shadowMu.Unlock()
		return
	}
	s.shadowRunning[key] = true
	s.shadowMu.Unlock()

	go func() {
		defer func() {
			s.shadowMu.Lock()
			delete(s.shadowRunning, key)
			s.shadowMu.Unlock()
		}()
		_ = s.shadowRunOnce(tenant, sess, r)
	}()
}

func (s *Server) shadowRunOnce(tenant control.Tenant, sess session, r *http.Request) error {
	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	cfg, err := s.control.TenantInferenceConfig(tenant.Slug)
	if err != nil {
		return err
	}
	if cfg == nil {
		return nil
	}

	marker, items, triggerAdded, err := s.shadowRopeSnapshot(db, tenant, sess, r)
	if err != nil {
		return err
	}
	if triggerAdded == 0 {
		return nil
	}

	headers := map[string]string{}
	if strings.TrimSpace(cfg.HeadersJSON) != "" {
		_ = json.Unmarshal([]byte(cfg.HeadersJSON), &headers)
	}
	client, err := inference.New(inference.Config{
		Provider: inference.ProviderKind(cfg.Provider),
		BaseURL:  cfg.BaseURL,
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		Headers:  headers,
		Timeout:  90 * time.Second,
	})
	if err != nil {
		return err
	}

	dev := strings.TrimSpace(`
You are an observe-only agent. Do not change system state or take external actions.
Only respond with tool calls. If you have nothing useful to say, call ui.no_action.
If you do message the user, call ui.message with a short, helpful note or question.
`)

	user := buildShadowRopePrompt(marker, items)

	tools := []inference.ToolDef{
		makeUINoActionTool(),
		makeUIMessageTool(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	res, err := client.Stream(ctx, inference.Request{
		Messages: []inference.Message{
			{Role: "developer", Content: dev},
			{Role: "user", Content: user},
		},
		Tools: tools,
	}, func(ev inference.StreamEvent) error {
		// For now, ignore streaming events. We'll surface them later in the rail if desired.
		return nil
	})
	if err != nil {
		return err
	}

	var calls []agent.FunctionCall
	for _, raw := range res.FunctionCalls {
		var c agent.FunctionCall
		if err := json.Unmarshal(raw, &c); err != nil {
			continue
		}
		calls = append(calls, c)
	}
	_ = applyUIFunctionCalls(db, sess.UserID, calls)
	return nil
}

func buildShadowRopePrompt(marker shadowRopeMarker, items []shadowRopeItem) string {
	var b strings.Builder
	b.WriteString("Event rope (timestamps are UTC):\n")
	if marker.BeforeLedgerEventID != 0 && strings.TrimSpace(marker.BeforeCreatedAt) != "" {
		b.WriteString("… context truncated before ledger_event_id=")
		b.WriteString(strconv.FormatInt(marker.BeforeLedgerEventID, 10))
		b.WriteString(" at ")
		b.WriteString(strings.TrimSpace(marker.BeforeCreatedAt))
		b.WriteString("\n")
	}
	for _, it := range items {
		ts := strings.TrimSpace(it.CreatedAt)
		if ts == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(ts)
		b.WriteString(" — ")
		b.WriteString(strings.TrimSpace(it.Narration))
		b.WriteString("\n")
	}
	b.WriteString("\nInstructions: Call ui.message or ui.no_action.\n")
	return b.String()
}

func makeUINoActionTool() inference.ToolDef {
	var t inference.ToolDef
	t.Type = "function"
	t.Function.Name = "ui.no_action"
	t.Function.Description = "Explicitly do nothing."
	t.Function.Parameters = json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false}`)
	t.Function.Strict = true
	return t
}

func makeUIMessageTool() inference.ToolDef {
	var t inference.ToolDef
	t.Type = "function"
	t.Function.Name = "ui.message"
	t.Function.Description = "Send a short message to the user (used for observations or clarifying questions)."
	t.Function.Parameters = json.RawMessage(`{
  "type":"object",
  "properties":{
    "text":{"type":"string","description":"Message to show the user."},
    "title":{"type":["string","null"],"description":"Optional short title."},
    "summary":{"type":["string","null"],"description":"Optional 1-line summary for the rail."}
  },
  "required":["text","title","summary"],
  "additionalProperties":false
}`)
	t.Function.Strict = true
	return t
}
