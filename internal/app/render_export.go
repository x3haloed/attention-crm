package app

import (
	"attention-crm/internal/control"
	"html/template"
	"strings"
)

func renderExportBody(tenant control.Tenant) template.HTML {
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)
	var b strings.Builder

	b.WriteString(`<div class="max-w-3xl mx-auto">`)
	b.WriteString(`<div class="flex items-center justify-between mb-6">`)
	b.WriteString(`<div>`)
	b.WriteString(`<h1 class="text-2xl font-semibold text-gray-900">Export</h1>`)
	b.WriteString(`<p class="mt-1 text-sm text-gray-600">Download your workspace data as CSV.</p>`)
	b.WriteString(`</div>`)
	b.WriteString(`<a class="text-sm font-medium text-blue-600 hover:text-blue-700 hover:underline" href="/t/` + tenantSlugEsc + `/app">Back to app</a>`)
	b.WriteString(`</div>`)

	b.WriteString(`<div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6 space-y-4">`)
	b.WriteString(exportRow("Contacts", "People and notes.", "/t/"+tenantSlugEsc+"/export/contacts.csv"))
	b.WriteString(exportRow("Interactions", "Timeline items (notes, calls, follow-ups).", "/t/"+tenantSlugEsc+"/export/interactions.csv"))
	b.WriteString(exportRow("Deals", "Opportunities and attached contacts.", "/t/"+tenantSlugEsc+"/export/deals.csv"))
	b.WriteString(`</div>`)

	b.WriteString(`<div class="mt-4 text-xs text-gray-500">Tip: open these in Excel/Sheets, or import into another CRM. Large text fields (notes/content) may include newlines.</div>`)
	b.WriteString(`</div>`)

	return template.HTML(b.String())
}

func exportRow(title, desc, href string) string {
	var b strings.Builder
	b.WriteString(`<div class="flex items-center justify-between gap-4 p-4 rounded-xl border border-gray-200">`)
	b.WriteString(`<div class="min-w-0">`)
	b.WriteString(`<div class="text-sm font-semibold text-gray-900">` + template.HTMLEscapeString(title) + `</div>`)
	b.WriteString(`<div class="text-sm text-gray-600 mt-0.5">` + template.HTMLEscapeString(desc) + `</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`<a class="shrink-0 h-10 px-4 inline-flex items-center rounded-xl bg-gray-900 text-white text-sm font-medium hover:bg-black" href="` + template.HTMLEscapeString(href) + `">Download</a>`)
	b.WriteString(`</div>`)
	return b.String()
}

