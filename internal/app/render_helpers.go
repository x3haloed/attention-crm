package app

import (
	"attention-crm/internal/tenantdb"
	"html/template"
	"strconv"
	"strings"
	"time"
)

func quickCaptureButton(title, subtitle, hoverClass, iconBgClass, iconBgHoverClass, iconClass, iconPath, intent string) string {
	intentAttr := ""
	if strings.TrimSpace(intent) != "" {
		intentAttr = ` data-omni-intent="` + template.HTMLEscapeString(strings.TrimSpace(intent)) + `"`
	}
	return `<button type="button"` + intentAttr + ` class="bg-white border border-gray-200 rounded-xl p-6 ` + hoverClass + ` transition-all duration-200 group">
	  <div class="flex flex-col items-center text-center space-y-3">
	    <div class="w-12 h-12 ` + iconBgClass + ` rounded-full flex items-center justify-center ` + iconBgHoverClass + `">
	      <svg class="w-5 h-5 ` + iconClass + `" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="` + iconPath + `"></path></svg>
	    </div>
	    <div>
	      <p class="text-sm font-medium text-gray-900">` + template.HTMLEscapeString(title) + `</p>
	      <p class="text-xs text-gray-500 mt-1">` + template.HTMLEscapeString(subtitle) + `</p>
	    </div>
	  </div>
	</button>`
}

func initials(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return "?"
	}
	if len(parts) == 1 {
		r := []rune(parts[0])
		if len(r) == 0 {
			return "?"
		}
		if len(r) == 1 {
			return strings.ToUpper(string(r[0]))
		}
		return strings.ToUpper(string(r[0:2]))
	}
	a := []rune(parts[0])
	b := []rune(parts[len(parts)-1])
	if len(a) == 0 || len(b) == 0 {
		return "?"
	}
	return strings.ToUpper(string([]rune{a[0], b[0]}))
}

func snippet(text string, max int) string {
	t := strings.TrimSpace(text)
	r := []rune(t)
	if len(r) <= max {
		return t
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}

func parseRFC3339(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t2, err2 := time.Parse(time.RFC3339, s)
		if err2 != nil {
			return time.Time{}, false
		}
		t = t2
	}
	return t, true
}

func relativeTime(createdAt string, now time.Time) string {
	t, ok := parseRFC3339(createdAt)
	if !ok {
		return createdAt
	}
	d := now.Sub(t)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " minutes ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " hours ago"
	case d < 48*time.Hour:
		return "Yesterday"
	case d < 7*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + " days ago"
	default:
		return t.Format("Jan 2, 2006")
	}
}

func attentionItemMeta(it tenantdb.Interaction, now time.Time) (bgClass, iconClass, meta, actionText string) {
	bgClass = "bg-amber-50 border border-amber-200"
	iconClass = "text-amber-600"
	meta = "Due soon"
	actionText = "Act"
	if it.DueAt.Valid {
		if dueT, ok := parseRFC3339(it.DueAt.String); ok {
			if dueT.Before(now) {
				bgClass = "bg-red-50 border border-red-200"
				iconClass = "text-red-600"
				actionText = "Act"
				over := now.Sub(dueT)
				if over < time.Hour {
					meta = "Overdue by " + strconv.Itoa(int(over.Minutes())) + " minutes"
				} else if over < 24*time.Hour {
					meta = "Overdue by " + strconv.Itoa(int(over.Hours())) + " hours"
				} else {
					meta = "Overdue by " + strconv.Itoa(int(over.Hours()/24)) + " days"
				}
				return
			}
			until := dueT.Sub(now)
			if until < time.Hour {
				meta = "Due in " + strconv.Itoa(int(until.Minutes())) + " minutes"
				actionText = "Start"
			} else if until < 24*time.Hour {
				meta = "Due in " + strconv.Itoa(int(until.Hours())) + " hours"
				actionText = "Start"
			} else {
				meta = "Due in " + strconv.Itoa(int(until.Hours()/24)) + " days"
				actionText = "Review"
			}
			return
		}
		meta = "Due: " + it.DueAt.String
		actionText = "Act"
	}
	return
}

func splitTitleDesc(content string) (string, string) {
	t := strings.TrimSpace(content)
	if t == "" {
		return "", ""
	}
	// Prefer splitting on first sentence boundary.
	for _, sep := range []string{". ", "\n"} {
		if idx := strings.Index(t, sep); idx > 0 {
			title := strings.TrimSpace(t[:idx+1])
			desc := strings.TrimSpace(t[idx+1:])
			if desc != "" {
				desc = strings.TrimLeft(desc, ". \n\t")
			}
			return title, desc
		}
	}
	return t, ""
}

func dueDisplay(dueAt string, now time.Time) string {
	t, ok := parseRFC3339(dueAt)
	if !ok {
		return dueAt
	}
	local := t.Local()
	if local.Year() == now.Local().Year() && local.YearDay() == now.Local().YearDay() {
		return "Today at " + local.Format("3:04 PM")
	}
	if local.After(now.Add(-48*time.Hour)) && local.Before(now.Add(48*time.Hour)) {
		// approximate tomorrow/yesterday.
		if local.After(now) && local.YearDay() == now.Local().Add(24*time.Hour).YearDay() {
			return "Tomorrow at " + local.Format("3:04 PM")
		}
	}
	return local.Format("Jan 2 at 3:04 PM")
}

func interactionIcon(interactionType, variant string) string {
	path := "M6 2h9l5 5v15a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Zm8 1.5V8h4.5L14 3.5Z"
	color := "text-blue-600"
	switch interactionType {
	case "call":
		path = "M6.62 10.79a15.053 15.053 0 0 0 6.59 6.59l2.2-2.2a1 1 0 0 1 1.01-.24c1.12.37 2.33.57 3.58.57a1 1 0 0 1 1 1V20a1 1 0 0 1-1 1C10.61 21 3 13.39 3 4a1 1 0 0 1 1-1h3.5a1 1 0 0 1 1 1c0 1.25.2 2.46.57 3.59a1 1 0 0 1-.24 1.01l-2.21 2.19Z"
		color = "text-green-600"
	case "email":
		path = "M20 4H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2Zm0 4-8 5L4 8V6l8 5 8-5v2Z"
		color = "text-purple-600"
	case "meeting":
		path = "M7 2v2H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2h-2V2h-2v2H9V2H7Zm12 18H5V9h14v11ZM7 11h5v5H7v-5Z"
		color = "text-indigo-600"
	default:
		color = "text-blue-600"
	}
	if variant == "due" {
		color = "text-amber-600"
	}
	if variant == "completed" {
		color = "text-green-600"
	}
	return `<svg class="w-4 h-4 ` + color + `" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="` + path + `"></path></svg>`
}

func dealStateBadge(state string) string {
	state = strings.ToLower(strings.TrimSpace(state))
	label := strings.Title(state)
	cls := "bg-gray-100 text-gray-800"
	switch state {
	case "open":
		label = "Open"
		cls = "bg-blue-50 text-blue-700"
	case "won":
		label = "Won"
		cls = "bg-green-50 text-green-700"
	case "lost":
		label = "Lost"
		cls = "bg-red-50 text-red-700"
	}
	return `<span class="text-xs font-medium rounded-full px-2 py-0.5 ` + cls + `">` + template.HTMLEscapeString(label) + `</span>`
}
