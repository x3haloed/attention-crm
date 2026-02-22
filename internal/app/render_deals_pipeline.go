package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"html/template"
	"strconv"
	"strings"
	"time"
)

func renderDealsPipelineHeader(tenant control.Tenant) template.HTML {
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)
	return template.HTML(`
<header id="header" class="bg-white border-b border-gray-200 px-4 py-4 lg:px-6">
  <div class="flex items-center justify-between max-w-4xl mx-auto">
    <div class="flex items-center space-x-4">
      <a href="/t/` + tenantSlugEsc + `/app" class="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg" aria-label="Back">
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M14.7 6.3 13.3 4.9 6.2 12l7.1 7.1 1.4-1.4L9 12l5.7-5.7Z"/></svg>
      </a>
      <div class="flex items-center space-x-2">
        <h1 class="text-xl lg:text-2xl font-semibold text-gray-900">Deals</h1>
        <span class="text-xs font-medium text-gray-500">Pipeline Lens</span>
      </div>
    </div>
  </div>
</header>`)
}

func renderDealsPipelineBody(tenant control.Tenant, rows []tenantdb.DealPipelineRow) template.HTML {
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)
	now := time.Now()

	var b strings.Builder
	b.WriteString(`<div class="space-y-6">`)

	b.WriteString(`<div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<div class="flex items-start justify-between gap-4 mb-4">`)
	b.WriteString(`<div>`)
	b.WriteString(`<div class="text-sm font-semibold text-gray-900">Open deals</div>`)
	b.WriteString(`<div class="text-sm text-gray-600">Sorted by attention: missing next step, due soon, then recent activity.</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`<a href="/t/` + tenantSlugEsc + `/app" class="text-sm font-medium text-blue-600 hover:text-blue-700 hover:underline">Use omnibar to create</a>`)
	b.WriteString(`</div>`)

	if len(rows) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No open deals yet.</div>`)
		b.WriteString(`</div></div>`)
		return template.HTML(b.String())
	}

	b.WriteString(`<div class="space-y-3">`)
	for _, r := range rows {
		d := r.Deal
		title := strings.TrimSpace(d.Title)
		if title == "" {
			title = "Untitled"
		}

		primaryLabel := strings.TrimSpace(r.PrimaryContactName)
		if r.PrimaryContactCompany != "" {
			primaryLabel = primaryLabel + " • " + r.PrimaryContactCompany
		}
		if primaryLabel == "" {
			primaryLabel = "Contact"
		}

		metaParts := make([]string, 0, 3)
		if strings.TrimSpace(d.StageLabel) != "" {
			metaParts = append(metaParts, d.StageLabel)
		}
		if r.ContactCount > 1 {
			metaParts = append(metaParts, primaryLabel+" +"+strconv.Itoa(r.ContactCount-1))
		} else {
			metaParts = append(metaParts, primaryLabel)
		}

		attn := ""
		attnClass := "text-gray-600"
		if strings.TrimSpace(d.NextStep) == "" {
			attn = "Next step needed"
			attnClass = "text-red-700"
		} else if d.NextStepDueAt.Valid && !d.NextStepCompleted.Valid {
			attn = "Due: " + dueDisplay(d.NextStepDueAt.String, now)
			attnClass = "text-amber-700"
			if dueT, ok := parseRFC3339(d.NextStepDueAt.String); ok && dueT.Before(now) {
				attnClass = "text-red-700"
			}
		}

		b.WriteString(`<a href="/t/` + tenantSlugEsc + `/deals/` + strconv.FormatInt(d.ID, 10) + `" class="block p-4 rounded-2xl border border-gray-200 hover:bg-gray-50">`)
		b.WriteString(`<div class="flex items-start justify-between gap-4">`)
		b.WriteString(`<div class="min-w-0">`)
		b.WriteString(`<div class="flex items-center gap-2">`)
		b.WriteString(`<div class="text-sm font-semibold text-gray-900 truncate">` + template.HTMLEscapeString(title) + `</div>`)
		b.WriteString(dealStateBadge(d.State))
		b.WriteString(`</div>`)

		if len(metaParts) > 0 {
			b.WriteString(`<div class="mt-1 text-xs text-gray-500">` + template.HTMLEscapeString(strings.Join(metaParts, " • ")) + `</div>`)
		}
		if strings.TrimSpace(d.NextStep) != "" {
			b.WriteString(`<div class="mt-2 text-sm text-gray-700">Next: ` + template.HTMLEscapeString(snippet(d.NextStep, 120)) + `</div>`)
		}
		if attn != "" {
			b.WriteString(`<div class="mt-2 text-xs font-medium ` + attnClass + `">` + template.HTMLEscapeString(attn) + `</div>`)
		}
		b.WriteString(`</div>`)

		rightMeta := ""
		if d.LastActivityAt != "" {
			rightMeta = "Updated " + relativeTime(d.LastActivityAt, now)
		}
		if rightMeta != "" {
			b.WriteString(`<div class="shrink-0 text-xs text-gray-500">` + template.HTMLEscapeString(rightMeta) + `</div>`)
		}
		b.WriteString(`</div>`)
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)

	return template.HTML(b.String())
}
