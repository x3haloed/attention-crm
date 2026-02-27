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

func renderTenantAppBody(
	tenant control.Tenant,
	userID int64,
	state appViewState,
	contacts []tenantdb.Contact,
	needsAttention []tenantdb.Interaction,
	needsDeals []tenantdb.Deal,
	recent []tenantdb.Interaction,
	agentPast []tenantdb.ActivityEvent,
	agentCurrent *tenantdb.ActivityEvent,
) template.HTML {
	var b strings.Builder
	now := time.Now()
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)

	if state.Flash != "" {
		b.WriteString(`<div class="mb-6 bg-blue-50 border border-blue-200 rounded-lg p-3 text-sm text-blue-900">` + template.HTMLEscapeString(state.Flash) + `</div>`)
	}

	// Possible duplicates (kept, but styled in the new system).
	if len(state.Duplicates) > 0 {
		b.WriteString(`<div class="bg-white rounded-xl shadow-sm border border-gray-200 p-6 mb-8">`)
		b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-3">Possible duplicates</div>`)
		b.WriteString(`<div class="space-y-2">`)
		for _, d := range state.Duplicates {
			label := d.Contact.Name
			if d.Contact.Company != "" {
				label += " • " + d.Contact.Company
			}
			b.WriteString(`<div class="flex items-center justify-between p-3 hover:bg-gray-50 rounded-lg">`)
			b.WriteString(`<a class="text-sm font-medium text-gray-900 hover:underline" href="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(d.Contact.ID, 10) + `">` + template.HTMLEscapeString(label) + `</a>`)
			b.WriteString(`<div class="text-xs text-gray-500">` + template.HTMLEscapeString(d.Reason) + `</div>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div></div>`)
	}

	// Home split layout: left dashboard (2/3) + right rail (1/3 placeholder).
	// Inline grid columns ensure this works even if Tailwind CSS has not been rebuilt yet.
	b.WriteString(`<div id="home-split-grid" class="grid gap-6 mt-6 w-full" style="grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);">`)
	b.WriteString(`<div id="home-main-column" class="min-w-0">`)

	// Universal action surface (kept in left 2/3 column).
	b.WriteString(string(renderOmniBar(tenant, state.UniversalText, "home")))

	// Quick capture section.
	b.WriteString(`<div id="quick-capture-section"><div class="grid grid-cols-4 gap-4">`)
	b.WriteString(quickCaptureButton("New Contact", "Add person or company", "hover:border-blue-300 hover:bg-blue-50", "bg-blue-100", "group-hover:bg-blue-200", "text-blue-600", "M12 5a3 3 0 1 0 0 6 3 3 0 0 0 0-6Zm-7 14c0-3.314 2.686-6 6-6h2c3.314 0 6 2.686 6 6v1H5v-1Zm13-6v-2h2V9h-2V7h-2v2h-2v2h2v2h2Z", "contact"))
	b.WriteString(quickCaptureButton("Log Call", "Record conversation", "hover:border-green-300 hover:bg-green-50", "bg-green-100", "group-hover:bg-green-200", "text-green-600", "M6.62 10.79a15.053 15.053 0 0 0 6.59 6.59l2.2-2.2a1 1 0 0 1 1.01-.24c1.12.37 2.33.57 3.58.57a1 1 0 0 1 1 1V20a1 1 0 0 1-1 1C10.61 21 3 13.39 3 4a1 1 0 0 1 1-1h3.5a1 1 0 0 1 1 1c0 1.25.2 2.46.57 3.59a1 1 0 0 1-.24 1.01l-2.21 2.19Z", "call"))
	b.WriteString(quickCaptureButton("Quick Note", "Capture thoughts", "hover:border-yellow-300 hover:bg-yellow-50", "bg-yellow-100", "group-hover:bg-yellow-200", "text-yellow-600", "M6 2h9l5 5v15a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Zm8 1.5V8h4.5L14 3.5Z", "note"))
	b.WriteString(quickCaptureButton("New Deal", "Track opportunity", "hover:border-purple-300 hover:bg-purple-50", "bg-purple-100", "group-hover:bg-purple-200", "text-purple-600", "M20 6h-3.586l-1.707-1.707A1 1 0 0 0 14 4H10a1 1 0 0 0-.707.293L7.586 6H4a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2Zm0 12H4V8h4l2-2h4l2 2h4v10Z", "deal"))
	b.WriteString(`</div></div>`)

	b.WriteString(`<div class="grid grid-cols-12 gap-6 mt-6" id="content-grid">`)

	// Needs Attention
	b.WriteString(`<div id="needs-attention-section" class="col-span-5"><div class="bg-white rounded-xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<div class="flex items-center justify-between mb-6"><h2 class="text-lg font-semibold text-gray-900">Needs Attention</h2>`)
	totalNeeds := len(needsAttention) + len(needsDeals)
	if totalNeeds > 0 {
		b.WriteString(`<span class="bg-red-100 text-red-800 text-xs font-medium px-2 py-1 rounded-full">` + strconv.Itoa(totalNeeds) + `</span>`)
	} else {
		b.WriteString(`<span class="text-xs font-medium text-green-700 bg-green-50 border border-green-200 px-2 py-1 rounded-full">All clear</span>`)
	}
	b.WriteString(`</div>`)

	if len(needsAttention) == 0 && len(needsDeals) == 0 {
		b.WriteString(`<div class="text-sm text-gray-700">No follow-ups or deals need attention.</div>`)
		b.WriteString(`<div class="mt-2 text-xs text-gray-500">Use the omnibar above to log a note, call, or create a deal.</div>`)
	} else {
		b.WriteString(`<div class="space-y-4">`)
		if len(needsAttention) == 0 {
			b.WriteString(`<div class="text-sm text-gray-700">No follow-ups due.</div>`)
		} else {
			for _, it := range needsAttention {
				bgClass, iconClass, meta, actionText := attentionItemMeta(it, now)
				b.WriteString(`<div class="flex items-start space-x-3 p-3 ` + bgClass + ` rounded-lg">`)
				b.WriteString(`<svg class="w-4 h-4 ` + iconClass + ` mt-1" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 8v5l4 2-1 1.732L10 14V8h2Zm0-6a10 10 0 1 0 0 20 10 10 0 0 0 0-20Z"/></svg>`)
				b.WriteString(`<div class="flex-1">`)
				b.WriteString(`<p class="text-sm font-medium text-gray-900">Follow up with ` + template.HTMLEscapeString(it.ContactName) + `</p>`)
				b.WriteString(`<p class="text-xs mt-1 ` + iconClass + `">` + template.HTMLEscapeString(meta) + `</p>`)
				b.WriteString(`</div>`)
				b.WriteString(`<a class="text-xs font-medium hover:underline ` + iconClass + `" href="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(it.ContactID, 10) + `">` + template.HTMLEscapeString(actionText) + `</a>`)
				b.WriteString(`</div>`)
			}
		}
		b.WriteString(`</div>`)

		if len(needsDeals) > 0 {
			b.WriteString(`<div class="mt-6 pt-6 border-t border-gray-100"></div>`)
			b.WriteString(`<div class="flex items-center justify-between mb-3">`)
			b.WriteString(`<div class="text-xs font-medium text-gray-500 uppercase tracking-wider">Deals</div>`)
			b.WriteString(`<a href="/t/` + tenantSlugEsc + `/deals" class="text-xs font-medium text-blue-600 hover:text-blue-700 hover:underline">View all</a>`)
			b.WriteString(`</div>`)
			b.WriteString(`<div class="space-y-3">`)
			for _, d := range needsDeals {
				title := strings.TrimSpace(d.Title)
				if title == "" {
					title = "Untitled"
				}
				meta := ""
				if strings.TrimSpace(d.NextStep) == "" {
					meta = "Next step needed"
				} else {
					meta = "Next: " + snippet(d.NextStep, 64)
				}
				if d.NextStepDueAt.Valid && d.NextStepCompleted.Valid == false {
					meta = meta + " • " + dueLabel(d.NextStepDueAt.String, now)
				} else if strings.TrimSpace(d.NextStep) != "" && staleLabel(d.LastActivityAt, now) != "" {
					meta = meta + " • " + staleLabel(d.LastActivityAt, now)
				}
				b.WriteString(`<a href="/t/` + tenantSlugEsc + `/deals/` + strconv.FormatInt(d.ID, 10) + `" class="block p-3 rounded-lg border border-gray-200 hover:bg-gray-50">`)
				b.WriteString(`<div class="flex items-start justify-between gap-3">`)
				b.WriteString(`<div class="min-w-0">`)
				b.WriteString(`<div class="text-sm font-medium text-gray-900 truncate">` + template.HTMLEscapeString(title) + `</div>`)
				b.WriteString(`<div class="mt-1 text-xs text-gray-600">` + template.HTMLEscapeString(meta) + `</div>`)
				b.WriteString(`</div>`)
				b.WriteString(`<div class="shrink-0">` + dealStateBadge(d.State) + `</div>`)
				b.WriteString(`</div>`)
				b.WriteString(`</a>`)
			}
			b.WriteString(`</div>`)
		} else {
			b.WriteString(`<div class="mt-6 pt-6 border-t border-gray-100"></div>`)
			b.WriteString(`<div class="flex items-center justify-between mb-2">`)
			b.WriteString(`<div class="text-xs font-medium text-gray-500 uppercase tracking-wider">Deals</div>`)
			b.WriteString(`<a href="/t/` + tenantSlugEsc + `/deals" class="text-xs font-medium text-blue-600 hover:text-blue-700 hover:underline">View all</a>`)
			b.WriteString(`</div>`)
			b.WriteString(`<div class="text-sm text-gray-700">No deals need attention.</div>`)
		}
	}
	b.WriteString(`</div></div>`)

	// Recent Interactions
	b.WriteString(`<div id="recent-interactions-section" class="col-span-7"><div class="bg-white rounded-xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<h2 class="text-lg font-semibold text-gray-900 mb-6">Recent Interactions</h2>`)
	if len(recent) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No interactions yet.</div>`)
	} else {
		b.WriteString(`<div class="space-y-4">`)
		for _, it := range recent {
			title, desc := splitTitleDesc(it.Content)
			b.WriteString(`<a href="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(it.ContactID, 10) + `" class="flex items-center space-x-4 p-3 hover:bg-gray-50 rounded-lg cursor-pointer">`)
			b.WriteString(`<div class="w-10 h-10 bg-blue-600 rounded-full flex items-center justify-center"><span class="text-white text-xs font-semibold">` + template.HTMLEscapeString(initials(it.ContactName)) + `</span></div>`)
			b.WriteString(`<div class="flex-1">`)
			b.WriteString(`<p class="text-sm font-medium text-gray-900">` + template.HTMLEscapeString(it.ContactName) + `</p>`)
			line := title
			if desc != "" {
				line = title + " — " + desc
			}
			b.WriteString(`<p class="text-xs text-gray-600 mt-1">` + template.HTMLEscapeString(snippet(line, 120)) + `</p>`)
			b.WriteString(`</div>`)
			b.WriteString(`<div class="text-xs text-gray-500">` + template.HTMLEscapeString(relativeTime(it.CreatedAt, now)) + `</div>`)
			b.WriteString(`</a>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></div>`)

	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	b.WriteString(`<style>
			@keyframes agent-typing { from { width: 0; } to { width: 100%; } }
			@keyframes agent-caret { 0%, 50% { border-color: transparent; } 51%, 100% { border-color: rgb(59 130 246); } }
			@keyframes agent-pulse-glow {
				0% { box-shadow: 0 0 0 0 rgba(34,197,94,0.35); }
				70% { box-shadow: 0 0 0 10px rgba(34,197,94,0); }
				100% { box-shadow: 0 0 0 0 rgba(34,197,94,0); }
			}
			.agent-typing-animation {
				display: inline-block;
				max-width: 100%;
			white-space: nowrap;
			overflow: hidden;
			border-right: 2px solid rgb(59 130 246);
			animation: agent-typing 3s steps(40, end) infinite, agent-caret 1s step-end infinite;
		}
		.agent-typing-paused { animation-play-state: paused; border-right-color: transparent; }
		.agent-pulse-glow { animation: agent-pulse-glow 1.6s infinite; }
		@media (prefers-reduced-motion: reduce) {
			.agent-typing-animation { animation: none; border-right-color: transparent; }
			.agent-pulse-glow { animation: none; }
		}
	</style>`)

	// Right rail: agent workspace scaffold (static layout only).
	b.WriteString(`<aside id="home-right-rail" class="min-w-0">`)
	b.WriteString(`<div class="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">`)
	b.WriteString(`<div class="flex flex-col" style="min-height: calc(100vh - 9rem);">`)
	b.WriteString(`<div class="p-6 border-b border-gray-200">`)
	b.WriteString(`<div class="flex flex-col items-center text-center">`)
	b.WriteString(`<div id="agent-avatar-host" class="w-56 h-56">`)
	b.WriteString(`<img src="/static/cute-chibi.svg?v=eyes-v2" alt="Chibi agent avatar" class="w-56 h-56 object-contain">`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="p-6 flex-1 min-h-0 bg-gray-50 overflow-auto">`)
	b.WriteString(renderAgentActionSpine(now, agentPast, agentCurrent))
	b.WriteString(`</div>`)

	b.WriteString(`<div class="mt-auto p-4 border-t border-gray-200 bg-white">`)
	b.WriteString(`<label for="agent-chat-input" class="block text-xs font-medium text-gray-700 mb-2">Chat with agent</label>`)
	b.WriteString(`<div class="rounded-xl border border-gray-300 focus-within:ring-2 focus-within:ring-blue-500 focus-within:border-blue-500">`)
	b.WriteString(`<textarea id="agent-chat-input" rows="4" class="w-full px-3 py-3 text-sm border-0 rounded-t-xl resize-y focus:outline-none" placeholder="Tell the agent what to do next..."></textarea>`)
	b.WriteString(`<div class="flex items-center justify-between px-3 py-2 border-t border-gray-200 bg-gray-50 rounded-b-xl">`)
	b.WriteString(`<div class="flex items-center gap-2">`)
	b.WriteString(`<button type="button" class="p-1.5 text-gray-500 hover:text-gray-700 hover:bg-gray-200 rounded-md" aria-label="Attach file">`)
	b.WriteString(`<svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M16.5 6.5v9a4.5 4.5 0 1 1-9 0v-10a3 3 0 1 1 6 0v9a1.5 1.5 0 0 1-3 0V7H9v7.5a3 3 0 0 0 6 0v-9a4.5 4.5 0 1 0-9 0v10a6 6 0 1 0 12 0v-9h-1.5z"/></svg>`)
	b.WriteString(`</button>`)
	b.WriteString(`<button type="button" class="p-1.5 text-gray-500 hover:text-gray-700 hover:bg-gray-200 rounded-md" aria-label="Voice input">`)
	b.WriteString(`<svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 14a3 3 0 0 0 3-3V6a3 3 0 1 0-6 0v5a3 3 0 0 0 3 3zm5-3a5 5 0 0 1-10 0H5a7 7 0 0 0 6 6.93V21h2v-3.07A7 7 0 0 0 19 11h-2z"/></svg>`)
	b.WriteString(`</button>`)
	b.WriteString(`<span class="text-[11px] text-gray-400">Enter to send</span>`)
	b.WriteString(`</div>`)
	b.WriteString(`<button id="agent-send-button" type="button" class="inline-flex items-center gap-1.5 bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium px-3 py-2 rounded-lg">`)
	b.WriteString(`<svg class="w-4 h-4" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M2.01 21 23 12 2.01 3 2 10l15 2-15 2z"/></svg>`)
	b.WriteString(`<span>Send</span>`)
	b.WriteString(`</button>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</aside>`)
	b.WriteString(renderHomeAgentWorkspaceScript())

	// Close split grid.
	b.WriteString(`</div>`)

	return template.HTML(b.String())
}

type emailDraftDetail struct {
	To      string   `json:"to"`
	Subject string   `json:"subject"`
	Body    []string `json:"body"`
}

func renderAgentActionSpine(now time.Time, past []tenantdb.ActivityEvent, current *tenantdb.ActivityEvent) string {
	var b strings.Builder

	// v0: until the real agent runtime is wired in, show a demo spine when empty so the UI isn't blank.
	// This avoids a "nothing rendered" failure mode during UX iteration.
	if len(past) == 0 && current == nil {
		past = []tenantdb.ActivityEvent{
			{Title: "Analyzed meeting notes", Summary: "Pulled out key points and constraints.", CreatedAt: now.Add(-2 * time.Minute).UTC().Format(time.RFC3339)},
			{Title: "Identified key action items", Summary: "Converted notes into next steps and owners.", CreatedAt: now.Add(-1 * time.Minute).UTC().Format(time.RFC3339)},
			{Title: "Gathered recipient context", Summary: "Checked prior interactions and tone.", CreatedAt: now.Add(-45 * time.Second).UTC().Format(time.RFC3339)},
		}
		detail, _ := json.Marshal(emailDraftDetail{
			To:      "john.doe@company.com",
			Subject: "Meeting Follow-up and Next Steps",
			Body: []string{
				"Dear John,",
				"Thank you for taking the time to meet with me yesterday. I wanted to follow up on our discussion about the upcoming project timeline.",
				"Looking forward to hearing your thoughts and moving this project forward together...",
			},
		})
		current = &tenantdb.ActivityEvent{
			Title:      "Email Draft",
			Summary:    "Drafting follow-up email from meeting notes.",
			DetailJSON: string(detail),
			CreatedAt:  now.Add(-15 * time.Second).UTC().Format(time.RFC3339),
		}
	}

	// Use a consistent "spine" layout regardless of whether there's an active event.
	b.WriteString(`<div id="email-drafting-area" class="flex-1 bg-gray-50 overflow-y-auto py-2">`)
	b.WriteString(`<div class="max-w-2xl mx-auto">`)
	b.WriteString(`<div id="action-spine" class="mb-6 sm:mb-8">`)
	b.WriteString(`<div class="relative">`)
	b.WriteString(`<div class="absolute left-4 top-0 bottom-0 w-0.5 bg-gradient-to-b from-blue-400 to-purple-400"></div>`)
	b.WriteString(`<div class="space-y-3 sm:space-y-4 min-h-[10rem]">`)

	colors := []struct {
		dot    string
		border string
	}{
		{dot: "bg-blue-500", border: "border-blue-100"},
		{dot: "bg-purple-500", border: "border-purple-100"},
		{dot: "bg-indigo-500", border: "border-indigo-100"},
	}

	// Past (oldest -> newest).
	for idx := len(past) - 1; idx >= 0; idx-- {
		ev := past[idx]
		c := colors[(len(past)-1-idx)%len(colors)]
		b.WriteString(`<div class="flex items-start space-x-4">`)
		b.WriteString(`<div class="flex-shrink-0 w-8 h-8 ` + c.dot + ` rounded-full flex items-center justify-center z-10 shadow-lg">`)
		b.WriteString(`<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4z"/></svg>`)
		b.WriteString(`</div>`)
		b.WriteString(`<div class="flex-1 bg-white rounded-lg shadow-md px-3 py-2 sm:px-4 sm:py-3 border ` + c.border + `">`)
		b.WriteString(`<div class="flex items-center justify-between gap-3">`)
		b.WriteString(`<span class="text-xs sm:text-sm font-medium text-gray-700">` + template.HTMLEscapeString(ev.Title) + `</span>`)
		b.WriteString(`<span class="text-xs text-gray-400">` + template.HTMLEscapeString(relativeTime(ev.CreatedAt, now)) + `</span>`)
		b.WriteString(`</div>`)
		if strings.TrimSpace(ev.Summary) != "" {
			b.WriteString(`<div class="mt-1 text-[11px] sm:text-xs text-gray-500">` + template.HTMLEscapeString(ev.Summary) + `</div>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(`</div>`)
	}

	if current != nil {
		// Current action node + rich view.
		b.WriteString(`<div class="flex items-start space-x-4">`)
		b.WriteString(`<div class="flex-shrink-0 w-8 h-8 bg-green-500 rounded-full flex items-center justify-center z-10 shadow-lg agent-pulse-glow">`)
		b.WriteString(`<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25Zm18-11.5a1 1 0 0 0 0-1.41l-1.34-1.34a1 1 0 0 0-1.41 0l-1.13 1.13 3.75 3.75L21 5.75Z"/></svg>`)
		b.WriteString(`</div>`)

		// Rich current view: email draft (v1).
		var detail emailDraftDetail
		_ = json.Unmarshal([]byte(current.DetailJSON), &detail)
		if strings.TrimSpace(detail.To) == "" {
			detail.To = "john.doe@company.com"
		}
		if strings.TrimSpace(detail.Subject) == "" {
			detail.Subject = "Meeting Follow-up and Next Steps"
		}
		if len(detail.Body) == 0 {
			detail.Body = []string{
				"Dear John,",
				"Thank you for taking the time to meet with me yesterday. I wanted to follow up on our discussion about the upcoming project timeline.",
				"Looking forward to hearing your thoughts and moving this project forward together...",
			}
		}

		b.WriteString(`<div class="bg-white rounded-lg shadow-lg p-4 sm:p-6 border border-gray-200 ml-12 w-full min-w-0">`)
		b.WriteString(`<div class="flex items-center justify-between mb-4">`)
		b.WriteString(`<h2 class="text-lg sm:text-xl font-semibold text-gray-800">` + template.HTMLEscapeString(current.Title) + `</h2>`)
		b.WriteString(`<div class="flex space-x-2">`)
		b.WriteString(`<div class="w-3 h-3 bg-red-400 rounded-full"></div>`)
		b.WriteString(`<div class="w-3 h-3 bg-yellow-400 rounded-full"></div>`)
		b.WriteString(`<div class="w-3 h-3 bg-green-400 rounded-full"></div>`)
		b.WriteString(`</div>`)
		b.WriteString(`</div>`)
		if strings.TrimSpace(current.Summary) != "" {
			b.WriteString(`<div class="text-xs text-gray-600 mb-4">` + template.HTMLEscapeString(current.Summary) + `</div>`)
		}

		b.WriteString(`<div class="space-y-4">`)
		b.WriteString(`<div class="flex items-center space-x-3">`)
		b.WriteString(`<label class="text-sm font-medium text-gray-600 w-16">To:</label>`)
		b.WriteString(`<div class="flex-1 border-b border-gray-200 pb-1 min-w-0"><span id="agent-to-line" class="text-gray-800 text-sm sm:text-base break-all">` + template.HTMLEscapeString(detail.To) + `</span></div>`)
		b.WriteString(`</div>`)
		b.WriteString(`<div class="flex items-center space-x-3">`)
		b.WriteString(`<label class="text-sm font-medium text-gray-600 w-16">Subject:</label>`)
		b.WriteString(`<div class="flex-1 border-b border-gray-200 pb-1 min-w-0"><span id="agent-subject-line" class="text-gray-800 text-sm sm:text-base">` + template.HTMLEscapeString(detail.Subject) + `</span></div>`)
		b.WriteString(`</div>`)

		b.WriteString(`<div class="mt-6">`)
		b.WriteString(`<div class="bg-gray-50 rounded-lg p-4 sm:p-6 min-h-48">`)
		b.WriteString(`<div id="email-body" class="space-y-3 sm:space-y-4 text-sm sm:text-base">`)
		for i, line := range detail.Body {
			if i == len(detail.Body)-1 {
				b.WriteString(`<p id="agent-typing-line" class="text-gray-800 agent-typing-animation">` + template.HTMLEscapeString(line) + `</p>`)
				continue
			}
			b.WriteString(`<p class="text-gray-800">` + template.HTMLEscapeString(line) + `</p>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(`</div>`)
		b.WriteString(`</div>`)

		// Reason/Evidence stubs.
		b.WriteString(`<div class="pt-4 border-t border-gray-100">`)
		b.WriteString(`<div class="flex items-center justify-between gap-3">`)
		b.WriteString(`<div class="text-xs text-gray-500">Reason and evidence available (stub)</div>`)
		b.WriteString(`<button type="button" disabled class="px-2.5 py-1.5 text-xs font-medium rounded-md border border-gray-200 text-gray-400 bg-white">Evidence</button>`)
		b.WriteString(`</div>`)
		b.WriteString(`</div>`)

		// Placeholder controls (Undo/Revise) for witness-paced commits later.
		b.WriteString(`<div class="pt-4 flex items-center justify-end gap-2">`)
		b.WriteString(`<button type="button" disabled class="px-3 py-1.5 text-xs font-medium rounded-md border border-gray-200 text-gray-400 bg-white">Undo</button>`)
		b.WriteString(`<button type="button" disabled class="px-3 py-1.5 text-xs font-medium rounded-md border border-gray-200 text-gray-400 bg-white">Revise</button>`)
		b.WriteString(`</div>`)

		b.WriteString(`</div>`)
		b.WriteString(`</div>`)
		b.WriteString(`</div>`)
		b.WriteString(`</div>`)
	}

	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	return b.String()
}

func renderHomeAgentWorkspaceScript() string {
	return `<script>
(() => {
  const host = document.getElementById('agent-avatar-host');
  const chatInput = document.getElementById('agent-chat-input');
  const sendButton = document.getElementById('agent-send-button');
  const subjectLine = document.getElementById('agent-subject-line');
  const typingLine = document.getElementById('agent-typing-line');
  if (!host || !chatInput || !subjectLine || !typingLine) return;

  let avatarSvg = null;
  let pupilL = null;
  let pupilR = null;
  let lidL = null;
  let lidR = null;
  const eyeL = { x: 278, y: 540 };
  const eyeR = { x: 605, y: 540 };
  const keyboardFocus = { x: 442, y: 740 };
  const cameraFocus = { x: 442, y: 520 };
  const maxOffset = 20;

  let chatFocused = false;
  let lastMouseMove = 0;
  const mouseWindowMs = 650;
  let mouseSvg = { x: keyboardFocus.x, y: keyboardFocus.y };
  let smoothTarget = { x: keyboardFocus.x, y: keyboardFocus.y };
  let pupilOffset = { x: 0, y: 0 };
  let nextBlinkAt = performance.now() + rand(1200, 4200);
  let blinkPhase = 0;
  let blinkT = 0;

  function rand(a, b) { return a + Math.random() * (b - a); }
  function clamp(v, lo, hi) { return Math.max(lo, Math.min(hi, v)); }
  function lerp(a, b, t) { return a + (b - a) * t; }

  function setTyping(typing) {
    typingLine.classList.toggle('agent-typing-paused', !typing);
  }

  function computeTarget() {
    const mouseActive = (performance.now() - lastMouseMove) < mouseWindowMs;
    if (mouseActive) return mouseSvg;
    return chatFocused ? cameraFocus : keyboardFocus;
  }

  function updateEyes(dt) {
    if (!pupilL || !pupilR) return;
    const target = computeTarget();
    const follow = 0.12;
    smoothTarget.x = lerp(smoothTarget.x, target.x, 1 - Math.pow(1 - follow, dt * 60));
    smoothTarget.y = lerp(smoothTarget.y, target.y, 1 - Math.pow(1 - follow, dt * 60));
    const mid = { x: (eyeL.x + eyeR.x) / 2, y: (eyeL.y + eyeR.y) / 2 };
    let dx = smoothTarget.x - mid.x;
    let dy = smoothTarget.y - mid.y;
    const dist = Math.hypot(dx, dy) || 1;
    const scale = Math.min(1, maxOffset / dist);
    dx *= scale;
    dy *= scale;
    const ease = 0.22;
    pupilOffset.x = lerp(pupilOffset.x, dx, 1 - Math.pow(1 - ease, dt * 60));
    pupilOffset.y = lerp(pupilOffset.y, dy, 1 - Math.pow(1 - ease, dt * 60));
    const transform = 'translate(' + pupilOffset.x + ',' + pupilOffset.y + ')';
    pupilL.setAttribute('transform', transform);
    pupilR.setAttribute('transform', transform);
  }

  function updateBlink(dt) {
    if (!lidL || !lidR) return;
    const now = performance.now();
    if (blinkPhase === 0 && now >= nextBlinkAt) {
      blinkPhase = 1;
      blinkT = 0;
    }
    if (blinkPhase === 0) {
      lidL.setAttribute('height', '0');
      lidR.setAttribute('height', '0');
      return;
    }
    const closeDur = 0.08;
    const openDur = 0.1;
    if (blinkPhase === 1) {
      blinkT += dt;
      const t = clamp(blinkT / closeDur, 0, 1);
      const h = lerp(0, 120, t);
      lidL.setAttribute('height', String(h));
      lidR.setAttribute('height', String(h));
      if (t >= 1) {
        blinkPhase = 2;
        blinkT = 0;
      }
    } else {
      blinkT += dt;
      const t = clamp(blinkT / openDur, 0, 1);
      const h = lerp(120, 0, t);
      lidL.setAttribute('height', String(h));
      lidR.setAttribute('height', String(h));
      if (t >= 1) {
        blinkPhase = 0;
        nextBlinkAt = now + rand(1600, 5200);
      }
    }
  }

  function mapMouseToSvg(event) {
    if (!avatarSvg) return;
    const viewBox = avatarSvg.viewBox && avatarSvg.viewBox.baseVal ? avatarSvg.viewBox.baseVal : null;
    if (!viewBox) return;
    const rect = avatarSvg.getBoundingClientRect();
    if (!rect.width || !rect.height) return;
    const rx = (event.clientX - rect.left) / rect.width;
    const ry = (event.clientY - rect.top) / rect.height;
    mouseSvg.x = viewBox.x + rx * viewBox.width;
    mouseSvg.y = viewBox.y + ry * viewBox.height;
  }

  let lastFrame = performance.now();
  function frame(now) {
    const dt = Math.min(0.033, (now - lastFrame) / 1000);
    lastFrame = now;
    updateEyes(dt);
    updateBlink(dt);
    requestAnimationFrame(frame);
  }

  chatInput.addEventListener('focus', () => {
    chatFocused = true;
    setTyping(false);
  });
  chatInput.addEventListener('blur', () => {
    chatFocused = false;
    setTimeout(() => {
      if (document.activeElement !== chatInput) setTyping(true);
    }, 80);
  });
  chatInput.addEventListener('keydown', (event) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      if (sendButton) sendButton.click();
    }
  });

  if (sendButton) {
    sendButton.addEventListener('click', () => {
      if (!chatInput.value.trim()) return;
      sendButton.classList.add('scale-95');
      setTimeout(() => {
        sendButton.classList.remove('scale-95');
        chatInput.value = '';
        chatInput.blur();
      }, 160);
    });
  }

  window.addEventListener('mousemove', (event) => {
    lastMouseMove = performance.now();
    mapMouseToSvg(event);
  }, { passive: true });

  fetch('/static/cute-chibi.svg?v=eyes-v2')
    .then((response) => response.text())
    .then((svgText) => {
      host.innerHTML = svgText;
      avatarSvg = host.querySelector('svg');
      if (!avatarSvg) return;
      avatarSvg.setAttribute('width', '100%');
      avatarSvg.setAttribute('height', '100%');
      avatarSvg.setAttribute('preserveAspectRatio', 'xMidYMid meet');
      pupilL = avatarSvg.querySelector('#pupilL');
      pupilR = avatarSvg.querySelector('#pupilR');
      lidL = avatarSvg.querySelector('#lidL');
      lidR = avatarSvg.querySelector('#lidR');
      setTyping(true);
      requestAnimationFrame(frame);
    })
    .catch(() => {
      setTyping(true);
    });
})();
</script>`
}
