package app

import (
	"attention-crm/internal/agent"
	"attention-crm/internal/control"
	"attention-crm/internal/inference"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type shadowLoopEventKind string

const (
	shadowLoopEventStart  shadowLoopEventKind = "start"
	shadowLoopEventSkip   shadowLoopEventKind = "skip"
	shadowLoopEventPrompt shadowLoopEventKind = "prompt"
	shadowLoopEventTool   shadowLoopEventKind = "tool"
	shadowLoopEventEnd    shadowLoopEventKind = "end"
)

type shadowLoopEvent struct {
	Kind shadowLoopEventKind
	Step int

	Forced   bool
	AgentKey string
	CILHead  string
	Messages []inference.Message
	Skip     *shadowLoopSkip

	ToolCall *agent.FunctionCall
	ToolOut  *shadowToolExecResult
}

type shadowLoopSkip struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// shadowRunLoop executes the shadow-mode inference loop, optionally emitting events for debugging.
// It returns nil error even if there was no work to do (e.g. no new ledger events and not forced).
func (s *Server) shadowRunLoop(ctx context.Context, tenant control.Tenant, sess session, r *http.Request, force bool, emit func(shadowLoopEvent)) error {
	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	agentKey := shadowSessionKey(r, sess, tenant)
	cilHead, _ := db.CompileCILHead(agentKey, 30)
	if emit != nil {
		emit(shadowLoopEvent{
			Kind:     shadowLoopEventStart,
			Forced:   force,
			AgentKey: agentKey,
			CILHead:  strings.TrimSpace(cilHead),
		})
	}

	cfg, err := s.control.TenantInferenceConfig(tenant.Slug)
	if err != nil {
		return err
	}
	if cfg == nil {
		if emit != nil {
			emit(shadowLoopEvent{
				Kind: shadowLoopEventSkip,
				Skip: &shadowLoopSkip{
					Code:    "inference_not_configured",
					Message: "No inference config found for this tenant. Configure a provider first.",
				},
			})
		}
		return nil
	}

	marker, items, triggerAdded, err := s.shadowRopeSnapshot(db, tenant, sess, r)
	if err != nil {
		return err
	}
	if triggerAdded == 0 && !force {
		if emit != nil {
			emit(shadowLoopEvent{
				Kind: shadowLoopEventSkip,
				Skip: &shadowLoopSkip{
					Code:    "no_new_ledger_events",
					Message: "No new ledger events since the last rope snapshot. Check “Force run” to run anyway.",
				},
			})
		}
		return nil
	}

	userName := ""
	if u, err := db.WebAuthnUserByID(sess.UserID); err == nil {
		userName = strings.TrimSpace(u.Name)
	}
	userFirstName := firstNameOrFallback(userName, "User")

	company := strings.TrimSpace(tenant.Name)
	if company == "" {
		company = tenant.Slug
	}

	dev := shadowDeveloperMessage(company, userFirstName)
	user := buildShadowRopePrompt(tenant, userFirstName, cilHead, marker, items)

	tools := []inference.ToolDef{
		makeUINoActionTool(),
		makeUIMessageTool(),
		makeCILAppendINVTool(),
		makeNotesAppendTool(),
		makeNotesSearchTool(),
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

	messages := []inference.Message{
		{Role: "developer", Content: dev},
		{Role: "user", Content: user},
	}

	const maxSteps = 6
	for step := 0; step < maxSteps; step++ {
		if emit != nil {
			emit(shadowLoopEvent{
				Kind:     shadowLoopEventPrompt,
				Step:     step,
				Messages: append([]inference.Message(nil), messages...),
			})
		}

		res, err := client.Stream(ctx, inference.Request{
			Messages:                 messages,
			Tools:                    tools,
			RequireToolCall:          true,
			DisableParallelToolCalls: true,
		}, func(ev inference.StreamEvent) error { return nil })
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
			if emit != nil {
				emit(shadowLoopEvent{
					Kind: shadowLoopEventSkip,
					Skip: &shadowLoopSkip{
						Code:    "no_tool_call_returned",
						Message: "Provider returned no tool call. Check model/tool configuration.",
					},
				})
			}
			return nil
		}
		if strings.TrimSpace(call.CallID) == "" {
			call.CallID = "call_shadow_" + strconv.Itoa(step)
		}

		execRes, err := executeShadowToolCall(db, agentKey, sess.UserID, call)
		if err != nil {
			return err
		}
		if emit != nil {
			tmpCall := call
			tmpOut := execRes
			emit(shadowLoopEvent{
				Kind:     shadowLoopEventTool,
				Step:     step,
				ToolCall: &tmpCall,
				ToolOut:  &tmpOut,
			})
		}
		if execRes.Terminal {
			if emit != nil {
				emit(shadowLoopEvent{Kind: shadowLoopEventEnd, Step: step})
			}
			return nil
		}

		argsJSON, _ := call.NormalizedArguments()
		toolCall := inference.ToolCall{ID: call.CallID, Type: "function"}
		toolCall.Function.Name = call.Name
		toolCall.Function.Arguments = string(argsJSON)
		messages = append(messages,
			inference.Message{Role: "assistant", ToolCalls: []inference.ToolCall{toolCall}},
			inference.Message{Role: "tool", ToolCallID: call.CallID, Content: execRes.Output},
		)
	}

	return errors.New("shadow loop exceeded max tool steps without terminal UI action")
}

func shadowDeveloperMessage(company string, userFirstName string) string {
	return strings.TrimSpace(`
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
}
