package app

import (
	"strings"
	"time"
)

func looksLikeContactName(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || len(trimmed) > 80 {
		return false
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 || len(parts) > 4 {
		return false
	}
	stopwords := map[string]struct{}{
		"call": {}, "email": {}, "meeting": {}, "meet": {}, "tomorrow": {}, "today": {}, "next": {}, "follow": {}, "up": {},
	}
	for _, p := range parts {
		if _, found := stopwords[strings.ToLower(p)]; found {
			return false
		}
		for _, r := range p {
			if !(r == '\'' || r == '-' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
				return false
			}
		}
	}
	return true
}

func parseDueSuggestionLocal(input string, now time.Time) (string, bool) {
	text := strings.ToLower(input)
	base := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())

	switch {
	case strings.Contains(text, "tomorrow"):
		return base.Add(24 * time.Hour).Format("2006-01-02T15:04"), true
	case strings.Contains(text, "today"):
		return base.Format("2006-01-02T15:04"), true
	default:
		return "", false
	}
}

func looksLikeNote(input string) bool {
	t := strings.ToLower(strings.TrimSpace(input))
	if t == "" {
		return false
	}
	// If it's longer than a plausible "name" query, assume it's a note.
	// This is intentionally biased toward "note" to avoid accidentally suggesting "Create contact: <sentence>".
	if len(strings.Fields(t)) >= 4 {
		return true
	}
	noteWords := []string{
		"call ",
		"email ",
		"meet ",
		"meeting ",
		"follow up",
		"follow-up",
		"mentioned",
		"discussed",
		"said",
		"remind",
		"tomorrow",
		"today",
	}
	for _, w := range noteWords {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

func extractContactQueryFromNote(input string) string {
	words := strings.Fields(strings.ToLower(input))
	if len(words) == 0 {
		return ""
	}
	verbs := map[string]struct{}{
		"call": {}, "email": {}, "meet": {}, "meeting": {}, "follow": {}, "up": {}, "with": {}, "remind": {}, "me": {},
		"mentioned": {}, "discussed": {}, "said": {}, "about": {}, "regarding": {}, "re": {},
	}
	stop := map[string]struct{}{"tomorrow": {}, "today": {}, "a": {}, "an": {}, "the": {}}

	out := make([]string, 0, len(words))
	for _, w := range words {
		if _, found := verbs[w]; found {
			continue
		}
		if _, found := stop[w]; found {
			continue
		}
		out = append(out, w)
	}
	return strings.TrimSpace(strings.Join(out, " "))
}
