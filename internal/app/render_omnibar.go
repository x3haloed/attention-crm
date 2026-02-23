package app

import (
	"attention-crm/internal/control"
	"html/template"
	"strings"
)

// renderOmniBar renders the omnibar UI + client-side palette behavior.
// variant:
// - "home": large card in main content (home screen)
// - "header": compact card suitable for placing inside the header on non-home screens
func renderOmniBar(tenant control.Tenant, value, variant string) template.HTML {
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)
	valueEsc := template.HTMLEscapeString(value)

	outerClass := "mb-8"
	cardPadding := "p-6"
	inputClass := "text-lg"
	if variant == "header" {
		outerClass = "py-4"
		cardPadding = "p-4"
		inputClass = "text-base"
	}

	var b strings.Builder

	// Universal action surface.
	b.WriteString(`<div id="universal-action-surface" class="` + outerClass + `"><div class="relative"><div id="omni-card" data-tenant-slug="` + tenantSlugEsc + `" class="relative bg-white rounded-2xl shadow-sm border border-gray-200 ` + cardPadding + `">`)
	b.WriteString(`<form id="omni-form" method="POST" action="/t/` + tenantSlugEsc + `/universal"><div class="flex items-center space-x-4">`)
	b.WriteString(`<svg class="w-5 h-5 text-gray-400" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" id="omni-icon"><path fill="currentColor" d="M416 208c0 45.9-14.9 88.3-40 122.7L502.6 457.4c12.5 12.5 12.5 32.8 0 45.3s-32.8 12.5-45.3 0L330.7 376c-34.4 25.2-76.8 40-122.7 40C93.1 416 0 322.9 0 208S93.1 0 208 0S416 93.1 416 208zM208 352a144 144 0 1 0 0-288 144 144 0 1 0 0 288z"></path></svg>`)
	b.WriteString(`<div id="omni-input-wrap" class="flex-1 flex flex-wrap items-center gap-2">`)
	b.WriteString(`<div id="omni-chips" class="flex flex-wrap items-center gap-2"></div>`)
	b.WriteString(`<input id="omni-input" name="q" type="text" value="` + valueEsc + `" placeholder="Search contacts, deals, or add a quick note..." class="min-w-[12rem] flex-1 ` + inputClass + ` bg-transparent border-none outline-none placeholder-gray-400 text-gray-900" autocomplete="off" spellcheck="false">`)
	b.WriteString(`</div>`)
	b.WriteString(`</div></form>`)

	b.WriteString(`<div id="search-suggestions" class="mt-4 space-y-1 border-t border-gray-100 pt-4 hidden"></div>`)
	b.WriteString(`</div></div></div>`)

	// Client-side omnibar palette (intentionally embedded to keep the app single-binary/self-host friendly).
	b.WriteString("<script>\n")
	b.WriteString(omnibarClientJS)
	b.WriteString("\n</script>")

	return template.HTML(b.String())
}
