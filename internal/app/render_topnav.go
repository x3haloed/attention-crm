package app

import (
	"attention-crm/internal/control"
	"html/template"
	"strings"
)

func renderTopNavHeader(tenant control.Tenant) template.HTML {
	return renderTopNavHeaderWithOmniBar(tenant, "")
}

func renderTopNavHeaderWithOmniBar(tenant control.Tenant, omni template.HTML) template.HTML {
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)
	var b strings.Builder
	b.WriteString(`<header id="header" class="bg-white border-b border-gray-200 px-4 sm:px-6 py-4">`)
	b.WriteString(`<div class="flex items-center justify-between max-w-7xl mx-auto">`)
	b.WriteString(`<div class="flex items-center space-x-8">`)
	b.WriteString(`<a href="/t/` + tenantSlugEsc + `/app" class="flex items-center space-x-2">`)
	b.WriteString(`<div class="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center">`)
	b.WriteString(`<svg class="w-4 h-4 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M13 2 3 14h7l-1 8 12-14h-7l1-6z"></path></svg>`)
	b.WriteString(`</div>`)
	b.WriteString(`<span class="text-xl font-semibold text-gray-900">Attention CRM</span>`)
	b.WriteString(`</a>`)
	b.WriteString(`<nav class="hidden sm:flex items-center space-x-4 text-sm">`)
	b.WriteString(`<a class="text-gray-600 hover:text-gray-900 font-medium" href="/t/` + tenantSlugEsc + `/app">Home</a>`)
	b.WriteString(`<a class="text-gray-600 hover:text-gray-900 font-medium" href="/t/` + tenantSlugEsc + `/deals">Deals</a>`)
	b.WriteString(`<a class="text-gray-600 hover:text-gray-900 font-medium" href="/t/` + tenantSlugEsc + `/members">Members</a>`)
	b.WriteString(`<a class="text-gray-600 hover:text-gray-900 font-medium" href="/t/` + tenantSlugEsc + `/export">Export</a>`)
	b.WriteString(`</nav>`)
	b.WriteString(`</div>`)
	b.WriteString(`<div class="flex items-center space-x-4">`)
	b.WriteString(`<button type="button" class="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg" aria-label="Notifications">`)
	b.WriteString(`<svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 22a2 2 0 0 0 2-2h-4a2 2 0 0 0 2 2zm6-6V11a6 6 0 0 0-5-5.91V4a1 1 0 1 0-2 0v1.09A6 6 0 0 0 6 11v5l-2 2v1h16v-1l-2-2z"></path></svg>`)
	b.WriteString(`</button>`)
	b.WriteString(`<div class="w-8 h-8 rounded-full bg-gray-200"></div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	if strings.TrimSpace(string(omni)) != "" {
		b.WriteString(`<div class="max-w-7xl mx-auto">`)
		b.WriteString(string(omni))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</header>`)
	return template.HTML(b.String())
}
