package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"html/template"
	"strconv"
	"strings"
)

func inviteStatus(inv tenantdb.Invite) string {
	if inv.RevokedAt.Valid {
		return "Revoked"
	}
	if inv.RedeemedAt.Valid {
		return "Redeemed"
	}
	if inv.StartedAt.Valid {
		return "Started"
	}
	return "Pending"
}

func renderMembersBody(tenant control.Tenant, userID int64, members []tenantdb.Member, invites []tenantdb.Invite) template.HTML {
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)

	var b strings.Builder
	b.WriteString(`<div class="max-w-4xl mx-auto">`)
	b.WriteString(`<div class="flex items-center justify-between mb-6">`)
	b.WriteString(`<div>`)
	b.WriteString(`<h1 class="text-2xl font-semibold text-gray-900">Members</h1>`)
	b.WriteString(`<p class="mt-1 text-sm text-gray-600">Manage workspace members and pending invites.</p>`)
	b.WriteString(`</div>`)
	b.WriteString(`<a class="text-sm font-medium text-blue-600 hover:text-blue-700 hover:underline" href="/t/` + tenantSlugEsc + `/app">Back to app</a>`)
	b.WriteString(`</div>`)

	// Members
	b.WriteString(`<div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6 mb-6">`)
	b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-4">Current members</div>`)
	if len(members) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No members found.</div>`)
	} else {
		b.WriteString(`<div class="divide-y divide-gray-100">`)
		for _, m := range members {
			role := "Member"
			if m.IsOwner {
				role = "Owner"
			}
			initial := "?"
			if s := strings.TrimSpace(m.Name); s != "" {
				initial = strings.ToUpper(string([]rune(s)[0]))
			}
			b.WriteString(`<div class="py-3 flex items-center gap-3">`)
			b.WriteString(`<div class="w-9 h-9 rounded-full bg-gray-100 flex items-center justify-center text-sm font-semibold text-gray-600">` + template.HTMLEscapeString(initial) + `</div>`)
			b.WriteString(`<div class="flex-1 min-w-0">`)
			b.WriteString(`<div class="flex items-center gap-2">`)
			b.WriteString(`<div class="text-sm font-medium text-gray-900 truncate">` + template.HTMLEscapeString(m.Name) + `</div>`)
			if m.IsOwner {
				b.WriteString(`<span class="text-xs font-medium rounded-full bg-blue-50 text-blue-700 px-2 py-0.5">Owner</span>`)
			}
			b.WriteString(`</div>`)
			b.WriteString(`<div class="text-xs text-gray-500 truncate">` + template.HTMLEscapeString(m.Email) + `</div>`)
			b.WriteString(`</div>`)
			b.WriteString(`<div class="text-xs text-gray-500">` + template.HTMLEscapeString(role) + `</div>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	// Create invite (reuse existing behavior; link only shown after create on /app today).
	b.WriteString(`<div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6 mb-6">`)
	b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-1">Invite a teammate</div>`)
	b.WriteString(`<div class="text-sm text-gray-600 mb-4">Creates an invite link (no SMTP). The link is shown once after creation.</div>`)
	b.WriteString(`<form method="POST" action="/t/` + tenantSlugEsc + `/invites" class="flex items-end gap-3">`)
	b.WriteString(`<div class="flex-1">`)
	b.WriteString(`<label class="block text-sm font-medium text-gray-700">Email</label>`)
	b.WriteString(`<input name="email" type="email" required class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" placeholder="teammate@company.com">`)
	b.WriteString(`</div>`)
	b.WriteString(`<button type="submit" class="h-10 px-5 rounded-xl bg-blue-600 text-white font-medium hover:bg-blue-700">Create invite</button>`)
	b.WriteString(`</form>`)
	b.WriteString(`</div>`)

	// Invites list
	b.WriteString(`<div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-4">Invites</div>`)
	if len(invites) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No invites yet.</div>`)
	} else {
		b.WriteString(`<div class="space-y-2">`)
		for _, inv := range invites {
			status := inviteStatus(inv)
			statusClass := "bg-gray-50 text-gray-700"
			if status == "Pending" {
				statusClass = "bg-yellow-50 text-yellow-800"
			} else if status == "Started" {
				statusClass = "bg-blue-50 text-blue-700"
			} else if status == "Redeemed" {
				statusClass = "bg-green-50 text-green-700"
			} else if status == "Revoked" {
				statusClass = "bg-red-50 text-red-700"
			}

			b.WriteString(`<div class="flex items-center justify-between gap-4 p-3 rounded-xl border border-gray-200">`)
			b.WriteString(`<div class="min-w-0">`)
			b.WriteString(`<div class="text-sm font-medium text-gray-900 truncate">` + template.HTMLEscapeString(inv.Email) + `</div>`)
			b.WriteString(`<div class="text-xs text-gray-500">Created: ` + template.HTMLEscapeString(inv.CreatedAt) + ` • Expires: ` + template.HTMLEscapeString(inv.ExpiresAt) + `</div>`)
			b.WriteString(`</div>`)
			b.WriteString(`<div class="flex items-center gap-3">`)
			b.WriteString(`<span class="text-xs font-medium rounded-full px-2 py-0.5 ` + statusClass + `">` + template.HTMLEscapeString(status) + `</span>`)
			if status == "Pending" || status == "Started" {
				b.WriteString(`<form method="POST" class="m-0" action="/t/` + tenantSlugEsc + `/invites/` + strconv.FormatInt(inv.ID, 10) + `/revoke">`)
				b.WriteString(`<button type="submit" class="text-sm font-medium text-red-600 hover:text-red-700 hover:underline">Revoke</button>`)
				b.WriteString(`</form>`)
			}
			b.WriteString(`</div>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`</div>`)

	return template.HTML(b.String())
}
