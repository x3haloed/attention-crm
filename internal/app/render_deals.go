package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"html/template"
	"strconv"
	"strings"
	"time"
)

func renderDealDeskBody(tenant control.Tenant, deal tenantdb.Deal, targets []tenantdb.Contact, events []tenantdb.DealEvent, flash string) template.HTML {
	now := time.Now()
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)
	var b strings.Builder

	if flash != "" {
		b.WriteString(`<div class="mb-6 bg-blue-50 border border-blue-200 rounded-lg p-3 text-sm text-blue-900">` + template.HTMLEscapeString(flash) + `</div>`)
	}

	// Header row
	b.WriteString(`<div class="flex items-start justify-between gap-4 mb-6">`)
	b.WriteString(`<div class="min-w-0">`)
	b.WriteString(`<a href="/t/` + tenantSlugEsc + `/app" class="text-sm text-gray-600 hover:text-gray-900 hover:underline">← Back</a>`)
	b.WriteString(`<div class="mt-2 flex items-center gap-3 flex-wrap">`)
	b.WriteString(`<h1 class="text-2xl font-semibold text-gray-900 truncate">` + template.HTMLEscapeString(deal.Title) + `</h1>`)
	b.WriteString(dealStateBadge(deal.State))
	b.WriteString(`</div>`)
	b.WriteString(`<div class="mt-1 text-xs text-gray-500">Last activity: ` + template.HTMLEscapeString(relativeTime(deal.LastActivityAt, now)) + `</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	// Targets
	b.WriteString(`<div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6 mb-6">`)
	b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-3">Targets</div>`)
	if len(targets) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No targets attached.</div>`)
	} else {
		b.WriteString(`<div class="flex flex-wrap gap-2">`)
		for _, c := range targets {
			label := c.Name
			if c.Company != "" {
				label += " • " + c.Company
			}
			b.WriteString(`<a class="text-sm font-medium px-3 py-1.5 rounded-full bg-gray-50 hover:bg-gray-100 border border-gray-200 text-gray-900" href="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(c.ID, 10) + `">` + template.HTMLEscapeString(label) + `</a>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	// Next step
	nextStepMissing := strings.TrimSpace(deal.NextStep) == ""
	nextCardCls := "bg-white"
	if nextStepMissing && strings.ToLower(strings.TrimSpace(deal.State)) == "open" {
		nextCardCls = "bg-amber-50"
	}
	b.WriteString(`<div class="` + nextCardCls + ` rounded-2xl shadow-sm border border-gray-200 p-6 mb-6">`)
	b.WriteString(`<div class="flex items-start justify-between gap-4">`)
	b.WriteString(`<div class="min-w-0">`)
	b.WriteString(`<div class="text-sm font-semibold text-gray-900">Next step</div>`)
	if nextStepMissing {
		b.WriteString(`<div class="mt-1 text-sm text-amber-900">Missing next step. Add one to keep this deal moving.</div>`)
	} else {
		b.WriteString(`<div class="mt-1 text-sm font-medium text-gray-900">` + template.HTMLEscapeString(deal.NextStep) + `</div>`)
		if deal.NextStepDueAt.Valid && !deal.NextStepCompleted.Valid {
			b.WriteString(`<div class="mt-1 text-xs text-gray-600">Due: ` + template.HTMLEscapeString(dueDisplay(deal.NextStepDueAt.String, now)) + `</div>`)
		}
		if deal.NextStepCompleted.Valid {
			b.WriteString(`<div class="mt-1 text-xs text-green-700">Completed</div>`)
		}
	}
	b.WriteString(`</div>`)
	b.WriteString(`<div class="flex items-center gap-2">`)
	if !nextStepMissing && !deal.NextStepCompleted.Valid && strings.ToLower(strings.TrimSpace(deal.State)) == "open" {
		b.WriteString(`<form method="POST" action="/t/` + tenantSlugEsc + `/deals/` + strconv.FormatInt(deal.ID, 10) + `/next-step/complete">`)
		b.WriteString(`<button type="submit" class="h-10 px-4 rounded-xl bg-blue-600 text-white text-sm font-medium hover:bg-blue-700">Mark done</button>`)
		b.WriteString(`</form>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	if strings.ToLower(strings.TrimSpace(deal.State)) == "open" {
		b.WriteString(`<form method="POST" action="/t/` + tenantSlugEsc + `/deals/` + strconv.FormatInt(deal.ID, 10) + `/next-step" class="mt-4">`)
		b.WriteString(`<label class="block text-sm font-medium text-gray-700">What’s the next step?</label>`)
		b.WriteString(`<input name="next_step" type="text" value="` + template.HTMLEscapeString(deal.NextStep) + `" class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" placeholder="e.g. Send proposal" />`)
		b.WriteString(`<label class="block text-sm font-medium text-gray-700 mt-3">Due (optional)</label>`)
		b.WriteString(`<input name="due_at" type="datetime-local" class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" />`)
		b.WriteString(`<div class="mt-4 flex justify-end">`)
		b.WriteString(`<button type="submit" class="h-10 px-5 rounded-xl bg-blue-600 text-white font-medium hover:bg-blue-700">Save next step</button>`)
		b.WriteString(`</div>`)
		b.WriteString(`</form>`)
	}
	b.WriteString(`</div>`)

	// Timeline + quick log
	b.WriteString(`<div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<div class="flex items-center justify-between mb-4">`)
	b.WriteString(`<div class="text-sm font-semibold text-gray-900">Timeline</div>`)
	b.WriteString(`</div>`)

	if strings.ToLower(strings.TrimSpace(deal.State)) == "open" {
		b.WriteString(`<form method="POST" action="/t/` + tenantSlugEsc + `/deals/` + strconv.FormatInt(deal.ID, 10) + `/events" class="mb-6">`)
		b.WriteString(`<div class="flex items-center gap-3">`)
		b.WriteString(`<select name="type" class="h-10 bg-white border border-gray-200 rounded-lg px-3 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500">`)
		for _, t := range []string{"note", "call", "email", "meeting"} {
			b.WriteString(`<option value="` + template.HTMLEscapeString(t) + `">` + template.HTMLEscapeString(strings.Title(t)) + `</option>`)
		}
		b.WriteString(`</select>`)
		b.WriteString(`<input name="content" type="text" class="flex-1 h-10 bg-white border border-gray-200 rounded-lg px-3 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" placeholder="What happened? Quick note, call summary, or next steps…" />`)
		b.WriteString(`<button type="submit" class="h-10 px-4 rounded-xl bg-gray-900 text-white text-sm font-medium hover:bg-black">Log</button>`)
		b.WriteString(`</div>`)
		b.WriteString(`</form>`)
	}

	if len(events) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No timeline events yet.</div>`)
	} else {
		b.WriteString(`<div class="space-y-3">`)
		for _, ev := range events {
			evType := strings.ToLower(strings.TrimSpace(ev.Type))
			icon := interactionIcon(evType, "")
			label := strings.Title(evType)
			if evType == "system" {
				icon = `<svg class="w-4 h-4 text-gray-500" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20Zm1 11H7v-2h6V7h2v6Z"/></svg>`
				label = "System"
			}
			b.WriteString(`<div class="flex items-start gap-3 p-3 rounded-xl border border-gray-200">`)
			b.WriteString(`<div class="mt-0.5">` + icon + `</div>`)
			b.WriteString(`<div class="min-w-0 flex-1">`)
			b.WriteString(`<div class="flex items-center justify-between gap-3">`)
			b.WriteString(`<div class="text-sm font-medium text-gray-900">` + template.HTMLEscapeString(label) + `</div>`)
			b.WriteString(`<div class="text-xs text-gray-500">` + template.HTMLEscapeString(relativeTime(ev.CreatedAt, now)) + `</div>`)
			b.WriteString(`</div>`)
			b.WriteString(`<div class="text-sm text-gray-700 mt-1 whitespace-pre-wrap">` + template.HTMLEscapeString(ev.Content) + `</div>`)
			b.WriteString(`</div>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	// Close outcome (lightweight, but better than hidden defaults)
	if strings.ToLower(strings.TrimSpace(deal.State)) == "open" {
		b.WriteString(`<div class="mt-6 bg-white rounded-2xl shadow-sm border border-gray-200 p-6">`)
		b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-3">Close</div>`)
		b.WriteString(`<div class="text-sm text-gray-600 mb-4">One sentence: what happened?</div>`)
		b.WriteString(`<form method="POST" action="/t/` + tenantSlugEsc + `/deals/` + strconv.FormatInt(deal.ID, 10) + `/close" class="space-y-3">`)
		b.WriteString(`<textarea name="outcome" rows="2" class="w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" placeholder="e.g. Signed annual contract after security review"></textarea>`)
		b.WriteString(`<div class="flex items-center gap-2 justify-end">`)
		b.WriteString(`<button name="state" value="lost" type="submit" class="h-10 px-4 rounded-xl bg-gray-900 text-white text-sm font-medium hover:bg-black">Lost</button>`)
		b.WriteString(`<button name="state" value="won" type="submit" class="h-10 px-4 rounded-xl bg-green-600 text-white text-sm font-medium hover:bg-green-700">Won</button>`)
		b.WriteString(`</div>`)
		b.WriteString(`</form>`)
		b.WriteString(`</div>`)
	} else {
		if deal.ClosedAt.Valid {
			b.WriteString(`<div class="mt-6 bg-white rounded-2xl shadow-sm border border-gray-200 p-6">`)
			b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-2">Closed</div>`)
			b.WriteString(`<div class="text-sm text-gray-700">` + template.HTMLEscapeString(deal.ClosedOutcome) + `</div>`)
			b.WriteString(`<div class="text-xs text-gray-500 mt-1">` + template.HTMLEscapeString(relativeTime(deal.ClosedAt.String, now)) + `</div>`)
			b.WriteString(`</div>`)
		}
	}

	return template.HTML(b.String())
}
