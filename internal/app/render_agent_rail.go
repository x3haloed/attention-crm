package app

import (
	"encoding/json"
	"html/template"
	"strings"
	"time"
)

type emailDraftDetail struct {
	To      string   `json:"to"`
	Subject string   `json:"subject"`
	Body    []string `json:"body"`
}

func renderAgentRail(now time.Time, past []spineEvent, current *spineEvent) template.HTML {
	var b strings.Builder

	b.WriteString(`<div class="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden">`)
	b.WriteString(`<div class="flex flex-col min-h-[calc(100vh-9rem)]">`)

	b.WriteString(`<div class="p-6 border-b border-gray-200">`)
	b.WriteString(`<div class="flex flex-col items-center text-center">`)
	b.WriteString(`<div id="agent-avatar-host" class="w-56 h-56">`)
	b.WriteString(`<img src="/static/cute-chibi.svg?v=eyes-v2" alt="Chibi agent avatar" class="w-56 h-56 object-contain">`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="p-6 flex-1 min-h-0 bg-gray-50 overflow-auto">`)
	b.WriteString(renderAgentActionSpine(now, past, current))
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

	return template.HTML(b.String())
}

func renderAgentActionSpine(now time.Time, past []spineEvent, current *spineEvent) string {
	var b strings.Builder

	// Use a consistent "spine" layout regardless of whether there's an active event.
	b.WriteString(`<div id="email-drafting-area" class="flex-1 bg-gray-50 overflow-y-auto py-2">`)
	b.WriteString(`<div class="max-w-2xl mx-auto">`)
	b.WriteString(`<div id="action-spine" class="mb-6 sm:mb-8">`)
	b.WriteString(`<div class="relative">`)
	b.WriteString(`<div class="absolute left-4 top-0 bottom-0 w-0.5 bg-gradient-to-b from-blue-400 to-purple-400"></div>`)
	b.WriteString(`<div class="space-y-3 sm:space-y-4 min-h-[10rem]">`)

	agentColors := []struct {
		dot    string
		border string
	}{
		{dot: "bg-blue-500", border: "border-blue-100"},
		{dot: "bg-purple-500", border: "border-purple-100"},
		{dot: "bg-indigo-500", border: "border-indigo-100"},
	}

	if len(past) == 0 && current == nil {
		b.WriteString(`<div class="flex items-start space-x-4">`)
		b.WriteString(`<div class="flex-shrink-0 w-8 h-8 bg-gray-200 rounded-full flex items-center justify-center z-10">`)
		b.WriteString(`<svg class="w-4 h-4 text-gray-500" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 22a10 10 0 1 0-10-10 10 10 0 0 0 10 10Zm1-11V7h-2v6h6v-2Z"/></svg>`)
		b.WriteString(`</div>`)
		b.WriteString(`<div class="flex-1 bg-white rounded-lg shadow-sm px-3 py-2 sm:px-4 sm:py-3 border border-gray-200">`)
		b.WriteString(`<div class="text-xs sm:text-sm font-medium text-gray-700">No agent activity yet</div>`)
		b.WriteString(`<div class="mt-1 text-[11px] sm:text-xs text-gray-500">When the agent proposes actions, they’ll appear here as an append-only spine.</div>`)
		b.WriteString(`</div>`)
		b.WriteString(`</div>`)
	}

	// Past (oldest -> newest).
	for idx := len(past) - 1; idx >= 0; idx-- {
		ev := past[idx]
		dot := "bg-gray-300"
		border := "border-gray-200"
		icon := `<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4z"/></svg>`

		if strings.EqualFold(strings.TrimSpace(ev.ActorKind), "agent") {
			c := agentColors[(len(past)-1-idx)%len(agentColors)]
			dot = c.dot
			border = c.border
		} else if strings.EqualFold(strings.TrimSpace(ev.ActorKind), "human") {
			dot = "bg-slate-400"
			border = "border-slate-200"
			icon = `<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 12a4 4 0 1 0-4-4 4 4 0 0 0 4 4Zm0 2c-4.42 0-8 2-8 4.5V21h16v-2.5c0-2.5-3.58-4.5-8-4.5Z"/></svg>`
		}

		b.WriteString(`<div class="flex items-start space-x-4">`)
		b.WriteString(`<div class="flex-shrink-0 w-8 h-8 ` + dot + ` rounded-full flex items-center justify-center z-10 shadow-lg">`)
		b.WriteString(icon)
		b.WriteString(`</div>`)
		b.WriteString(`<div class="flex-1 bg-white rounded-lg shadow-md px-3 py-2 sm:px-4 sm:py-3 border ` + border + `">`)
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
			idAttr := ``
			classes := `text-gray-800`
			if i == len(detail.Body)-1 {
				idAttr = ` id="agent-typing-line"`
				classes = classes + ` agent-typing-animation`
			}
			b.WriteString(`<p` + idAttr + ` class="` + classes + `">` + template.HTMLEscapeString(line) + `</p>`)
		}
		b.WriteString(`</div>`)
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

	return b.String()
}
