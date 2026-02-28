package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
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

	return template.HTML(b.String())
}
