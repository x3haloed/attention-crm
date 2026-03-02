package app

import (
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
		s.shadowPending[key] = true
		s.shadowMu.Unlock()
		return
	}
	s.shadowRunning[key] = true
	s.shadowMu.Unlock()

	go func() {
		defer func() {
			s.shadowMu.Lock()
			delete(s.shadowPending, key)
			delete(s.shadowRunning, key)
			s.shadowMu.Unlock()
		}()

		// Coalesce bursts: if more requests arrive while a run is in-flight, run again after completion.
		for i := 0; i < 5; i++ {
			_ = s.shadowRunOnce(tenant, sess, r)

			s.shadowMu.Lock()
			pending := s.shadowPending[key]
			s.shadowPending[key] = false
			s.shadowMu.Unlock()

			if !pending {
				return
			}
		}
	}()
}

func (s *Server) shadowRunOnce(tenant control.Tenant, sess session, r *http.Request) error {
	// This runs in a goroutine after the request returns; do NOT use r.Context() (it cancels immediately).
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return s.shadowRunLoop(ctx, tenant, sess, r, false, nil)
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
