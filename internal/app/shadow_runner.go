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

	agentKey := shadowSessionKey(r, sess, tenant)

	userName := ""
	if u, err := db.WebAuthnUserByID(sess.UserID); err == nil {
		userName = strings.TrimSpace(u.Name)
	}
	userFirstName := firstNameOrFallback(userName, "User")

	cilHead, _ := db.CompileCILHead(agentKey, 30)

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

	company := strings.TrimSpace(tenant.Name)
	if company == "" {
		company = tenant.Slug
	}

	dev := strings.TrimSpace(`
You are learning to become a fully autonomous agent for ` + company + ` inside the Attention CRM product.
You are shadowing your human, ` + userFirstName + `.

Your job: observe incoming ledger events, infer intent, and fill in missing context by asking clarifying questions and recording durable learning.

You have two private memory outlets:
- CIL invariants via cil.append_inv (high value learning; strict schema; compiled head is injected into each prompt).
- Plain self-notes via notes.append / notes.search (fact recall; NOT the same as learning).

Heuristics:
- Ask a question when: an intent is unclear; a decision has consequences; a follow-up is implied but not specified; information looks inconsistent; an external effect occurred (e.g. an email was sent) and you need to confirm intent.
- Do ui.no_action when: the event is self-explanatory and you have no clarifying questions.
- Record a CIL invariant when: you observe a generalizable causal constraint that should change future autonomous behavior.
  - Required fields: id, name, trigger, because, if_not, scope, rev, src (plus optional: seen, last, ex).
  - Avoid "instruction smell": remove should/must; if it collapses, rewrite as because+if_not.
  - If you can't state a concrete if_not consequence, do NOT write an invariant; consider a plain self-note instead.
  - Do NOT write an invariant when: the event was routine; you're writing a reminder/checklist; the lesson won't recur; you can't state if_not concretely.
  - Smell tests:
    1) Remove "should" — if it collapses, it was an instruction.
    2) Argue with it — strengthen ex or if_not if it's easy to dispute.
    3) "Says who?" — if the answer isn't "this happened", rewrite with evidence/signature.

Output rules:
- You may call multiple tools, but you MUST end with a terminal UI tool:
  - Terminal: ui.message OR ui.no_action
  - Non-terminal: cil.append_inv, notes.append, notes.search
- Do not output plain text.
- If you call ui.message: ask a single clarifying question in 1–2 short sentences; use human language; do not mention internal event types or ids.
`)

	user := buildShadowRopePrompt(tenant, userFirstName, cilHead, marker, items)

	tools := []inference.ToolDef{
		makeUINoActionTool(),
		makeUIMessageTool(),
		makeCILAppendINVTool(),
		makeNotesAppendTool(),
		makeNotesSearchTool(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	messages := []inference.Message{
		{Role: "developer", Content: dev},
		{Role: "user", Content: user},
	}

	const maxSteps = 6
	for step := 0; step < maxSteps; step++ {
		res, err := client.Stream(ctx, inference.Request{
			Messages: messages,
			Tools:    tools,
			// Shadow mode is tool-call-only; force a tool call if provider supports it.
			RequireToolCall:          true,
			DisableParallelToolCalls: true,
		}, func(ev inference.StreamEvent) error {
			// For now, ignore streaming events. We'll surface them later in the rail if desired.
			return nil
		})
		if err != nil {
			return err
		}

		var call agent.FunctionCall
		found := false
		for _, raw := range res.FunctionCalls {
			if err := json.Unmarshal(raw, &call); err == nil {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		if strings.TrimSpace(call.CallID) == "" {
			call.CallID = "call_shadow_" + strconv.Itoa(step)
		}

		execRes, err := executeShadowToolCall(db, agentKey, sess.UserID, call)
		if err != nil {
			return err
		}
		if execRes.Terminal {
			return nil
		}

		// Feed tool calls + outputs back to the model (chat-completions compatible).
		argsJSON, _ := call.NormalizedArguments()
		toolCall := inference.ToolCall{ID: call.CallID, Type: "function"}
		toolCall.Function.Name = call.Name
		toolCall.Function.Arguments = string(argsJSON)
		messages = append(messages,
			inference.Message{Role: "assistant", ToolCalls: []inference.ToolCall{toolCall}},
			inference.Message{Role: "tool", ToolCallID: call.CallID, Content: execRes.Output},
		)
	}

	// Max steps reached without a terminal tool.
	return nil
}

func buildShadowRopePrompt(tenant control.Tenant, userFirstName string, cilHead string, marker shadowRopeMarker, items []shadowRopeItem) string {
	var b strings.Builder
	cilHead = strings.TrimSpace(cilHead)
	if cilHead != "" {
		b.WriteString("CIL (active invariants):\n")
		b.WriteString(cilHead)
		b.WriteString("\n\n")
	}
	b.WriteString("Here's everything that occurred recently (UTC, newest last):\n")
	if name := strings.TrimSpace(tenant.Name); name != "" {
		b.WriteString("Workspace: ")
		b.WriteString(name)
		b.WriteString("\n")
	}

	if marker.BeforeLedgerEventID != 0 && strings.TrimSpace(marker.BeforeCreatedAt) != "" {
		b.WriteString("… context truncated before ")
		b.WriteString(strings.TrimSpace(marker.BeforeCreatedAt))
		b.WriteString(" (ledger_event_id=")
		b.WriteString(strconv.FormatInt(marker.BeforeLedgerEventID, 10))
		b.WriteString(")\n\n")
	}

	for i, it := range items {
		ts := strings.TrimSpace(it.CreatedAt)
		if ts == "" {
			continue
		}
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(") ")
		b.WriteString(ts)
		b.WriteString(" — ")
		b.WriteString(strings.TrimSpace(it.Narration))
		b.WriteString("\n")
	}

	justIdx := -1
	for i := len(items) - 1; i >= 0; i-- {
		if strings.EqualFold(items[i].ActorKind, "human") {
			justIdx = i
			break
		}
	}
	if justIdx == -1 && len(items) > 0 {
		justIdx = len(items) - 1
	}
	if justIdx >= 0 {
		it := items[justIdx]
		if strings.TrimSpace(userFirstName) == "" {
			userFirstName = "User"
		}
		b.WriteString("\n")
		b.WriteString(userFirstName)
		b.WriteString(" just performed the following action:\n")
		b.WriteString(strings.TrimSpace(it.CreatedAt))
		b.WriteString(" — ")
		b.WriteString(strings.TrimSpace(it.Narration))
		b.WriteString("\n")
		if strings.TrimSpace(it.Detail) != "" {
			b.WriteString("\nContext:\n")
			for _, line := range strings.Split(it.Detail, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				b.WriteString("- ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\nNow respond by calling tools as needed, but end with exactly one terminal UI tool:\n- ui.message (ask a clarifying question)\n- ui.no_action (if nothing useful)\n")
	return b.String()
}

func firstNameOrFallback(fullName, fallback string) string {
	name := strings.TrimSpace(fullName)
	if name == "" {
		return fallback
	}
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return fallback
	}
	return parts[0]
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

func makeCILAppendINVTool() inference.ToolDef {
	var t inference.ToolDef
	t.Type = "function"
	t.Function.Name = "cil.append_inv"
	t.Function.Description = "Append a strict CIL invariant (INV). Use this only for generalizable causal constraints. Do not use for plain factual notes."
	t.Function.Parameters = json.RawMessage(`{
  "type":"object",
  "properties":{
    "id":{"type":"string","description":"Unique invariant id, e.g. INV-ACME-001."},
    "name":{"type":"string","description":"3–6 word label."},
    "trigger":{"type":"string","description":"Situation that activates this invariant."},
    "because":{"type":"string","description":"Causal reality — what is true about the world."},
    "if_not":{"type":"string","description":"Observable consequence of ignoring this."},
    "scope":{"type":"string","description":"Where this applies."},
    "seen":{"type":["integer","null"],"description":"Optional count of observations."},
    "last":{"type":["string","null"],"description":"Optional date of last observation (YYYY-MM-DD)."},
    "ex":{"type":["string","null"],"description":"Optional concrete exemplar (8–15 words)."},
    "rev":{"type":"string","description":"active or superseded_by INV-YYYY."},
    "src":{"type":"string","description":"Provenance pointer, e.g. chain#Entry@commit."}
  },
  "required":["id","name","trigger","because","if_not","scope","seen","last","ex","rev","src"],
  "additionalProperties":false
}`)
	t.Function.Strict = true
	return t
}

func makeNotesAppendTool() inference.ToolDef {
	var t inference.ToolDef
	t.Type = "function"
	t.Function.Name = "notes.append"
	t.Function.Description = "Append a plain self-note for later factual recall. Not for invariants."
	t.Function.Parameters = json.RawMessage(`{
  "type":"object",
  "properties":{
    "text":{"type":"string","description":"A short factual note the agent may want to recall later."}
  },
  "required":["text"],
  "additionalProperties":false
}`)
	t.Function.Strict = true
	return t
}

func makeNotesSearchTool() inference.ToolDef {
	var t inference.ToolDef
	t.Type = "function"
	t.Function.Name = "notes.search"
	t.Function.Description = "Search the agent's self-notes by substring match and return recent matches."
	t.Function.Parameters = json.RawMessage(`{
  "type":"object",
  "properties":{
    "query":{"type":"string","description":"Search query string."},
    "limit":{"type":["integer","null"],"description":"Max results (default 20)."}
  },
  "required":["query","limit"],
  "additionalProperties":false
}`)
	t.Function.Strict = true
	return t
}
