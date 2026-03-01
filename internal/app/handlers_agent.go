package app

import (
	"attention-crm/internal/agent"
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type applyToolCallsRequest struct {
	// FunctionCalls are tool calls emitted by the model. Only known ui.* tools are applied.
	FunctionCalls []agent.FunctionCall `json:"function_calls"`
}

type uiMessageArgs struct {
	Text   string `json:"text"`
	Title  string `json:"title,omitempty"`
	Summary string `json:"summary,omitempty"`
}

func applyUIFunctionCalls(db *tenantdb.Store, actorUserID int64, calls []agent.FunctionCall) int {
	applied := 0
	for _, c := range calls {
		if strings.TrimSpace(strings.ToLower(c.Type)) != "function_call" {
			continue
		}
		switch strings.TrimSpace(c.Name) {
		case "ui.no_action":
			applied++

		case "ui.message":
			argsJSON, err := c.NormalizedArguments()
			if err != nil {
				continue
			}
			var args uiMessageArgs
			_ = json.Unmarshal(argsJSON, &args)
			text := strings.TrimSpace(args.Text)
			if text == "" {
				continue
			}
			title := strings.TrimSpace(args.Title)
			if title == "" {
				title = "Agent message"
			}
			summary := strings.TrimSpace(args.Summary)
			if summary == "" {
				summary = snippet(text, 220)
			}
			detail, _ := json.Marshal(map[string]any{
				"kind": "message",
				"text": text,
			})
			_, _ = db.AppendAgentSpineEvent(actorUserID, tenantdb.AgentSpineEventInput{
				Status:     "done",
				Title:      title,
				Summary:    summary,
				DetailJSON: string(detail),
			})
			applied++
		}
	}
	return applied
}

func (s *Server) handleAgentToolCalls(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	var req applyToolCallsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	applied := applyUIFunctionCalls(db, sess.UserID, req.FunctionCalls)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"applied": applied,
	})
}
