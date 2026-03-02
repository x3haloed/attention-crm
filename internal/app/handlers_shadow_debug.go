package app

import (
	"attention-crm/internal/control"
	"bufio"
	"context"
	"encoding/json"
	"html/template"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleShadowTestPage(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	csrf, err := s.ensureCSRFCookie(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	userName := ""
	if u, err := db.WebAuthnUserByID(sess.UserID); err == nil {
		userName = strings.TrimSpace(u.Name)
	}
	userFirst := firstNameOrFallback(userName, "User")
	agentKey := shadowSessionKey(r, sess, tenant)
	cilHead, _ := db.CompileCILHead(agentKey, 30)

	esc := func(s string) string { return template.HTMLEscapeString(s) }

	body := template.HTML(`
<div class="max-w-4xl mx-auto px-4 py-6 lg:px-6">
  <div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6">
    <div class="flex items-start justify-between gap-6">
      <div>
        <h1 class="text-xl font-semibold text-gray-900">Shadow test</h1>
        <p class="mt-1 text-sm text-gray-600">Manually trigger shadow-mode inference and inspect the full prompt/tool loop transcript live.</p>
      </div>
      <a class="text-sm font-medium text-blue-700 hover:text-blue-800 hover:underline" href="/t/` + esc(tenant.Slug) + `/agent/infer">Inference config</a>
    </div>

    <div class="mt-4 rounded-xl border border-gray-200 bg-gray-50 p-4 text-sm text-gray-700">
      <div><span class="font-medium">Human:</span> ` + esc(userFirst) + `</div>
      <div class="mt-1"><span class="font-medium">Agent key:</span> <span class="font-mono text-xs">` + esc(agentKey) + `</span></div>
    </div>

    <div class="mt-6 flex flex-col sm:flex-row gap-3 sm:items-center sm:justify-between">
      <div class="text-sm text-gray-600" id="shadow-status"></div>
      <div class="flex items-center gap-3">
        <label class="inline-flex items-center gap-2 text-sm text-gray-700">
          <input id="shadow-force" type="checkbox" class="rounded border-gray-300 text-blue-600 focus:ring-blue-500" />
          Force run (even if no new ledger events)
        </label>
        <button type="button" id="shadow-backfill" class="rounded-xl bg-white px-4 py-2 text-sm font-medium text-gray-900 border border-gray-300 hover:bg-gray-50">Backfill ledger</button>
        <button type="button" id="shadow-run" class="rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700">Run now</button>
      </div>
    </div>
    <div class="mt-2 text-xs text-gray-500" id="shadow-backfill-status"></div>

    <div class="mt-6 grid grid-cols-1 gap-4">
      <div class="rounded-2xl border border-gray-200 bg-gray-50 p-4">
        <div class="text-sm font-medium text-gray-800">Injected CIL head</div>
        <pre class="mt-3 whitespace-pre-wrap text-xs text-gray-800 font-mono">` + esc(strings.TrimSpace(cilHead)) + `</pre>
      </div>
      <div class="rounded-2xl border border-gray-200 bg-gray-50 p-4">
        <div class="flex items-center justify-between">
          <div class="text-sm font-medium text-gray-800">Latest rope snapshot</div>
          <button type="button" id="shadow-refresh-rope" class="text-xs font-medium text-gray-700 hover:text-gray-900 hover:underline">Refresh</button>
        </div>
        <pre id="shadow-rope" class="mt-3 whitespace-pre-wrap text-xs text-gray-800 font-mono"></pre>
      </div>
      <div class="rounded-2xl border border-gray-200 bg-gray-50 p-4">
        <div class="flex items-center justify-between">
          <div class="text-sm font-medium text-gray-800">Live transcript</div>
          <button type="button" id="shadow-clear" class="text-xs font-medium text-gray-700 hover:text-gray-900 hover:underline">Clear</button>
        </div>
        <pre id="shadow-trace" class="mt-3 whitespace-pre-wrap text-xs text-gray-800 font-mono min-h-20"></pre>
      </div>
    </div>
  </div>
</div>
`)

	s.renderTenantAppPage(w, r, tenant, db, pageData{
		Title:      "Shadow test",
		TenantSlug: tenant.Slug,
		MainID:     "main-content",
		Body:       body,
		CSRFToken:  csrf,
	})
}

func (s *Server) handleShadowRunNow(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}

	body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	var req struct {
		Force bool `json:"force"`
	}
	if strings.TrimSpace(string(body)) != "" {
		_ = json.Unmarshal(body, &req)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)
	bw := bufio.NewWriterSize(w, 16<<10)
	flush := func() {
		_ = bw.Flush()
		if flusher != nil {
			flusher.Flush()
		}
	}
	send := func(event string, data any) {
		_, _ = bw.WriteString("event: " + event + "\n")
		b, _ := json.Marshal(data)
		_, _ = bw.WriteString("data: " + string(b) + "\n\n")
		flush()
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	err := s.shadowRunLoop(ctx, tenant, sess, r, req.Force, func(ev shadowLoopEvent) {
		switch ev.Kind {
		case shadowLoopEventStart:
			send("shadow.start", map[string]any{
				"forced":    ev.Forced,
				"agent_key": ev.AgentKey,
				"cil_head":  ev.CILHead,
			})
		case shadowLoopEventSkip:
			send("shadow.skip", map[string]any{
				"code":    ev.Skip.Code,
				"message": ev.Skip.Message,
			})
		case shadowLoopEventPrompt:
			send("shadow.prompt", map[string]any{
				"step":     ev.Step,
				"messages": ev.Messages,
			})
		case shadowLoopEventTool:
			send("shadow.tool", map[string]any{
				"step":      ev.Step,
				"tool_call": ev.ToolCall,
				"tool_out":  ev.ToolOut,
			})
		case shadowLoopEventEnd:
			send("shadow.end", map[string]any{
				"step": ev.Step,
			})
		}
	})
	if err != nil {
		send("shadow.error", map[string]any{"message": err.Error()})
	}
	send("shadow.done", map[string]any{"ok": err == nil})
}
