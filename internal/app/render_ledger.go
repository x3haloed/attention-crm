package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"encoding/json"
	"html/template"
	"strconv"
	"strings"
	"time"
)

type ledgerFilterState struct {
	ActorKind  string
	Op         string
	EntityType string
	EntityID   *int64
	Limit      int
}

func renderLedgerTimelineBody(tenant control.Tenant, events []tenantdb.LedgerEvent, f ledgerFilterState) template.HTML {
	var b strings.Builder
	now := time.Now()
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)

	b.WriteString(`<div class="flex items-start justify-between gap-4 mb-6">`)
	b.WriteString(`<div>`)
	b.WriteString(`<h1 class="text-xl font-semibold text-gray-900">Mutual Ledger</h1>`)
	b.WriteString(`<p class="mt-1 text-sm text-gray-600">Append-only event stream (human + agent). Projections are rebuildable.</p>`)
	b.WriteString(`</div>`)
	b.WriteString(`<a class="text-sm font-medium text-blue-700 hover:text-blue-800 hover:underline" href="/t/` + tenantSlugEsc + `/app">Back to Home</a>`)
	b.WriteString(`</div>`)

	// Filters
	b.WriteString(`<div class="bg-white border border-gray-200 rounded-xl p-4 shadow-sm mb-6">`)
	b.WriteString(`<form method="GET" action="/t/` + tenantSlugEsc + `/ledger" class="grid grid-cols-12 gap-3 items-end">`)

	b.WriteString(`<div class="col-span-12 sm:col-span-3">`)
	b.WriteString(`<label class="block text-xs font-medium text-gray-700 mb-1" for="ledger-actor">Actor</label>`)
	b.WriteString(`<select id="ledger-actor" name="actor" class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm">`)
	writeOpt := func(val, label string) {
		sel := ""
		if strings.EqualFold(strings.TrimSpace(f.ActorKind), val) {
			sel = " selected"
		}
		b.WriteString(`<option value="` + template.HTMLEscapeString(val) + `"` + sel + `>` + template.HTMLEscapeString(label) + `</option>`)
	}
	writeOpt("", "Any")
	writeOpt("human", "Human")
	writeOpt("agent", "Agent")
	writeOpt("system", "System")
	b.WriteString(`</select>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="col-span-12 sm:col-span-3">`)
	b.WriteString(`<label class="block text-xs font-medium text-gray-700 mb-1" for="ledger-entity">Entity</label>`)
	b.WriteString(`<input id="ledger-entity" name="entity" value="` + template.HTMLEscapeString(f.EntityType) + `" placeholder="contact, interaction, agent_spine…" class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm" />`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="col-span-12 sm:col-span-2">`)
	b.WriteString(`<label class="block text-xs font-medium text-gray-700 mb-1" for="ledger-id">ID</label>`)
	idVal := ""
	if f.EntityID != nil {
		idVal = strconv.FormatInt(*f.EntityID, 10)
	}
	b.WriteString(`<input id="ledger-id" name="id" value="` + template.HTMLEscapeString(idVal) + `" placeholder="123" class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm" />`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="col-span-12 sm:col-span-3">`)
	b.WriteString(`<label class="block text-xs font-medium text-gray-700 mb-1" for="ledger-op">Op</label>`)
	b.WriteString(`<input id="ledger-op" name="op" value="` + template.HTMLEscapeString(f.Op) + `" placeholder="contact.created" class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm" />`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="col-span-12 sm:col-span-1">`)
	b.WriteString(`<label class="block text-xs font-medium text-gray-700 mb-1" for="ledger-limit">Limit</label>`)
	b.WriteString(`<input id="ledger-limit" name="limit" value="` + strconv.Itoa(f.Limit) + `" class="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm" />`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="col-span-12 flex items-center gap-3 mt-1">`)
	b.WriteString(`<button type="submit" class="inline-flex items-center bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium px-3 py-2 rounded-lg">Apply</button>`)
	b.WriteString(`<a class="text-sm text-gray-600 hover:text-gray-900 hover:underline" href="/t/` + tenantSlugEsc + `/ledger">Reset</a>`)
	b.WriteString(`</div>`)

	b.WriteString(`</form>`)
	b.WriteString(`</div>`)

	// Timeline
	if len(events) == 0 {
		b.WriteString(`<div class="bg-white border border-gray-200 rounded-xl p-6 shadow-sm">`)
		b.WriteString(`<div class="text-sm font-medium text-gray-900">No events found.</div>`)
		b.WriteString(`<div class="mt-1 text-sm text-gray-600">Try loosening the filters, or generate some activity in the app.</div>`)
		b.WriteString(`</div>`)
		return template.HTML(b.String())
	}

	b.WriteString(`<div class="space-y-3">`)
	for _, ev := range events {
		meta := strings.ToUpper(ev.ActorKind) + " • " + ev.Op + " • " + ev.EntityType
		if ev.EntityID.Valid {
			meta += "#" + strconv.FormatInt(ev.EntityID.Int64, 10)
		}
		b.WriteString(`<div class="bg-white border border-gray-200 rounded-xl shadow-sm overflow-hidden">`)
		b.WriteString(`<div class="p-4">`)
		b.WriteString(`<div class="flex items-start justify-between gap-3">`)
		b.WriteString(`<div class="min-w-0">`)
		b.WriteString(`<div class="text-[11px] text-gray-500">` + template.HTMLEscapeString(meta) + `</div>`)
		b.WriteString(`<div class="mt-1 text-sm font-medium text-gray-900 break-words">Event ` + strconv.FormatInt(ev.ID, 10) + `</div>`)
		if strings.TrimSpace(ev.Reason) != "" {
			b.WriteString(`<div class="mt-1 text-sm text-gray-700 break-words">` + template.HTMLEscapeString(ev.Reason) + `</div>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(`<div class="text-xs text-gray-400 shrink-0">` + template.HTMLEscapeString(relativeTime(ev.CreatedAt, now)) + `</div>`)
		b.WriteString(`</div>`)

		b.WriteString(`<details class="mt-3">`)
		b.WriteString(`<summary class="cursor-pointer text-sm text-blue-700 hover:text-blue-800 select-none">Details</summary>`)
		b.WriteString(`<div class="mt-3 grid grid-cols-12 gap-3">`)

		// Payload
		b.WriteString(`<div class="col-span-12 lg:col-span-7">`)
		b.WriteString(`<div class="text-xs font-medium text-gray-700 mb-1">Payload</div>`)
		b.WriteString(`<pre class="text-xs bg-gray-50 border border-gray-200 rounded-lg p-3 overflow-auto">` + template.HTMLEscapeString(prettyJSON(ev.PayloadJSON)) + `</pre>`)
		b.WriteString(`</div>`)

		// Links
		b.WriteString(`<div class="col-span-12 lg:col-span-5">`)
		b.WriteString(`<div class="text-xs font-medium text-gray-700 mb-1">Links</div>`)
		b.WriteString(`<div class="space-y-2 text-sm">`)
		b.WriteString(`<div class="text-xs text-gray-600">Created at: <span class="font-mono text-gray-800">` + template.HTMLEscapeString(ev.CreatedAt) + `</span></div>`)
		if ev.CausedByEventID.Valid {
			b.WriteString(`<div class="text-xs text-gray-600">Caused by: <span class="font-mono text-gray-800">` + strconv.FormatInt(ev.CausedByEventID.Int64, 10) + `</span></div>`)
		}
		if ev.ReplacesEventID.Valid {
			b.WriteString(`<div class="text-xs text-gray-600">Replaces: <span class="font-mono text-gray-800">` + strconv.FormatInt(ev.ReplacesEventID.Int64, 10) + `</span></div>`)
		}
		if ev.InverseOfEventID.Valid {
			b.WriteString(`<div class="text-xs text-gray-600">Inverse of: <span class="font-mono text-gray-800">` + strconv.FormatInt(ev.InverseOfEventID.Int64, 10) + `</span></div>`)
		}
		if ev.IdempotencyKey.Valid && strings.TrimSpace(ev.IdempotencyKey.String) != "" {
			b.WriteString(`<div class="text-xs text-gray-600">Idempotency: <span class="font-mono text-gray-800 break-all">` + template.HTMLEscapeString(ev.IdempotencyKey.String) + `</span></div>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(`</div>`)

		// Evidence
		if strings.TrimSpace(ev.EvidenceJSON) != "" {
			b.WriteString(`<div class="col-span-12">`)
			b.WriteString(`<div class="text-xs font-medium text-gray-700 mb-1">Evidence</div>`)
			b.WriteString(`<pre class="text-xs bg-gray-50 border border-gray-200 rounded-lg p-3 overflow-auto">` + template.HTMLEscapeString(prettyJSON(ev.EvidenceJSON)) + `</pre>`)
			b.WriteString(`</div>`)
		}

		b.WriteString(`</div>`)
		b.WriteString(`</details>`)

		b.WriteString(`</div>`)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	return template.HTML(b.String())
}

func prettyJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return raw
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return raw
	}
	return string(b)
}

