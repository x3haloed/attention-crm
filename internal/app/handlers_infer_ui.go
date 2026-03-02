package app

import (
	"attention-crm/internal/control"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
)

func (s *Server) handleInferConfigPage(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
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

	cfg, err := s.control.TenantInferenceConfig(tenant.Slug)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	provider := ""
	baseURL := ""
	model := ""
	headersPretty := ""
	if cfg != nil {
		provider = cfg.Provider
		baseURL = cfg.BaseURL
		model = cfg.Model
		if strings.TrimSpace(cfg.HeadersJSON) != "" {
			var v any
			if json.Unmarshal([]byte(cfg.HeadersJSON), &v) == nil {
				if b, err := json.MarshalIndent(v, "", "  "); err == nil {
					headersPretty = string(b)
				}
			}
		}
	}
	if strings.TrimSpace(headersPretty) == "" {
		headersPretty = "{}"
	}

	csrf, err := s.ensureCSRFCookie(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	esc := func(s string) string { return template.HTMLEscapeString(s) }

	body := template.HTML(`
<div class="max-w-4xl mx-auto px-4 py-6 lg:px-6">
  <div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6">
    <div class="flex items-start justify-between gap-6">
      <div>
        <h1 class="text-xl font-semibold text-gray-900">Inference</h1>
        <p class="mt-1 text-sm text-gray-600">Configure the tenant’s inference backend (OpenAI/OpenRouter/LM Studio) and run a streaming test.</p>
      </div>
      <a class="text-sm font-medium text-blue-700 hover:text-blue-800 hover:underline" href="/t/` + esc(tenant.Slug) + `/ledger">Open ledger</a>
    </div>

    <div class="mt-6 grid grid-cols-1 md:grid-cols-2 gap-4">
      <div class="rounded-xl border border-gray-200 bg-gray-50 p-4">
        <div class="text-xs font-medium text-gray-700">CSRF token (copy)</div>
        <div class="mt-2 font-mono text-xs text-gray-800 break-all">` + esc(csrf) + `</div>
        <div class="mt-2 text-xs text-gray-600">
          For curl, <span class="font-mono">X-CSRF-Token</span> must exactly match the <span class="font-mono">attention_csrf</span> cookie value.
        </div>
      </div>
      <div class="rounded-xl border border-gray-200 bg-gray-50 p-4">
        <div class="text-xs font-medium text-gray-700">LM Studio base URL</div>
        <div class="mt-2 text-xs text-gray-600">Use the OpenAI-compatible endpoint: <span class="font-mono">http://127.0.0.1:1234</span> + <span class="font-mono">/v1/chat/completions</span>.</div>
        <div class="mt-2 text-xs text-gray-600">Provider: <span class="font-mono">lmstudio</span></div>
      </div>
    </div>

    <form id="infer-config-form" class="mt-6 space-y-4" autocomplete="off">
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label class="block text-sm font-medium text-gray-700" for="infer-provider">Provider</label>
          <select id="infer-provider" class="mt-1 w-full rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500">
            <option value="">Select…</option>
            <option value="openai" ` + selectedAttr(provider, "openai") + `>OpenAI</option>
            <option value="openrouter" ` + selectedAttr(provider, "openrouter") + `>OpenRouter</option>
            <option value="lmstudio" ` + selectedAttr(provider, "lmstudio") + `>LM Studio</option>
          </select>
          <div class="mt-1 text-xs text-gray-500">This controls which adapter is used; all are streamed as OpenAI-style SSE.</div>
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-700" for="infer-model">Model</label>
          <input id="infer-model" class="mt-1 w-full rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" value="` + esc(model) + `" placeholder="e.g. gpt-4o-mini or a local model id" />
        </div>
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-700" for="infer-base-url">Base URL</label>
        <input id="infer-base-url" class="mt-1 w-full rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" value="` + esc(baseURL) + `" placeholder="e.g. https://api.openai.com or http://127.0.0.1:1234" />
        <div class="mt-1 text-xs text-gray-500">Don’t include a trailing slash.</div>
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-700" for="infer-api-key">API key</label>
        <input id="infer-api-key" class="mt-1 w-full rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500" value="" placeholder="Leave blank to keep existing" />
      </div>
      <div>
        <label class="block text-sm font-medium text-gray-700" for="infer-headers">Extra headers (JSON)</label>
        <textarea id="infer-headers" rows="6" class="mt-1 w-full rounded-xl border border-gray-200 bg-white px-3 py-2 font-mono text-xs focus:outline-none focus:ring-2 focus:ring-blue-500" spellcheck="false">` + esc(headersPretty) + `</textarea>
        <div class="mt-1 text-xs text-gray-500">Optional (e.g. OpenRouter headers).</div>
      </div>

      <div class="flex flex-col sm:flex-row gap-3 sm:items-center sm:justify-between">
        <div class="text-sm text-gray-600" id="infer-config-status"></div>
        <div class="flex items-center gap-3">
          <button type="button" id="infer-stream-test" class="rounded-xl border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-800 hover:bg-gray-50">Streaming test</button>
          <button type="submit" class="rounded-xl bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700">Save</button>
        </div>
      </div>
    </form>

    <div class="mt-6 rounded-2xl border border-gray-200 bg-gray-50 p-4">
      <div class="flex items-center justify-between">
        <div class="text-sm font-medium text-gray-800">Streaming output</div>
        <button type="button" id="infer-stream-clear" class="text-xs font-medium text-gray-700 hover:text-gray-900 hover:underline">Clear</button>
      </div>
      <pre id="infer-stream-output" class="mt-3 whitespace-pre-wrap text-xs text-gray-800 font-mono min-h-16"></pre>
    </div>
  </div>
</div>
`)

	s.renderTenantAppPage(w, r, tenant, db, pageData{
		Title:      "Inference",
		TenantSlug: tenant.Slug,
		MainID:     "main-content",
		Body:       body,
		CSRFToken:  csrf,
	})
}

func selectedAttr(current, want string) string {
	if strings.EqualFold(strings.TrimSpace(current), strings.TrimSpace(want)) {
		return `selected`
	}
	return ``
}
