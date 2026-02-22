package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleUniversalInput(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	query := strings.TrimSpace(r.FormValue("q"))
	if query == "" {
		s.handleApp(w, r, tenant, appViewState{Flash: "Universal input is empty."})
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if looksLikeNote(query) {
		contactQuery := extractContactQueryFromNote(query)
		hints := []tenantdb.Contact{}
		if contactQuery != "" {
			hints, _ = db.SearchContacts(contactQuery, 10)
		}
		if len(hints) == 0 {
			hints, _ = db.ContactOptions()
			if len(hints) > 30 {
				hints = hints[:30]
			}
		}
		dueLocal, _ := parseDueSuggestionLocal(query, time.Now())

		s.handleApp(w, r, tenant, appViewState{
			Flash:         "This looks like a note.",
			UniversalText: query,
			SearchResults: hints,
			SuggestNote: &noteSuggestion{
				Content:      query,
				DueAtLocal:   dueLocal,
				ContactHints: hints,
			},
		})
		return
	}

	matches, err := db.SearchContacts(query, 10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(matches) == 1 && strings.EqualFold(strings.TrimSpace(matches[0].Name), query) {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/contacts/"+strconv.FormatInt(matches[0].ID, 10), http.StatusSeeOther)
		return
	}
	if len(matches) > 0 {
		s.handleApp(w, r, tenant, appViewState{
			Flash:         "Search results for \"" + query + "\".",
			UniversalText: query,
			SearchResults: matches,
		})
		return
	}

	if looksLikeContactName(query) {
		if err := db.CreateContact(query, "", "", "", ""); err != nil {
			s.handleApp(w, r, tenant, appViewState{Flash: "Could not create contact: " + err.Error(), UniversalText: query})
			return
		}
		createdMatches, _ := db.SearchContacts(query, 1)
		if len(createdMatches) == 1 {
			http.Redirect(w, r, "/t/"+tenant.Slug+"/contacts/"+strconv.FormatInt(createdMatches[0].ID, 10), http.StatusSeeOther)
			return
		}
		s.handleApp(w, r, tenant, appViewState{Flash: "Contact created from universal input.", UniversalText: query})
		return
	}

	s.handleApp(w, r, tenant, appViewState{
		Flash:         "No contact matched. If this is a follow-up note, choose a contact in Log Interaction.",
		UniversalText: query,
	})
}
