package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func inferInteractionTypeFromText(q string) string {
	t := strings.ToLower(strings.TrimSpace(q))
	switch {
	case strings.HasPrefix(t, "call ") || strings.Contains(t, " call "):
		return "call"
	case strings.HasPrefix(t, "email ") || strings.Contains(t, " email "):
		return "email"
	case strings.HasPrefix(t, "meet ") || strings.Contains(t, " meet ") || strings.Contains(t, "meeting"):
		return "meeting"
	default:
		return "note"
	}
}

func omniBuildActions(now time.Time, q string, matches []tenantdb.Contact, contactOptions []tenantdb.Contact) []omniAction {
	var actions []omniAction

	if looksLikeNote(q) {
		itype := inferInteractionTypeFromText(q)
		dueLocal, hasDue := parseDueSuggestionLocal(q, now)

		candidates := matches
		if len(candidates) == 0 {
			candidates = contactOptions
		}

		limit := 2
		if len(candidates) < limit {
			limit = len(candidates)
		}
		for i := 0; i < limit; i++ {
			act := omniAction{
				Type:            "log_interaction",
				ContactID:       candidates[i].ID,
				ContactName:     candidates[i].Name,
				InteractionType: itype,
				Content:         q,
			}
			if hasDue {
				act.DueAt = dueLocal
			}
			actions = append(actions, act)
		}

		// Always offer an explicit resolver that starts a "pick entity" flow.
		// (MVP: contacts only, but this row is intentionally generalized.)
		pick := omniAction{
			Type:            "pick_entity",
			InteractionType: itype,
			Content:         q,
		}
		if hasDue {
			pick.DueAt = dueLocal
		}
		actions = append(actions, pick)
	}

	if looksLikeContactName(q) {
		actions = append(actions, omniAction{
			Type: "create_contact",
			Name: q,
		})
	}

	return actions
}

func (s *Server) handleOmni(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"version":  2,
			"open":     false,
			"query":    "",
			"rows":     []map[string]any{},
			"contacts": []omniContact{},
			"actions":  []omniAction{},
		})
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	searchQ := q
	if looksLikeNote(q) {
		if cq := extractContactQueryFromNote(q); cq != "" {
			searchQ = cq
		}
	}
	matches, _ := db.SearchContacts(searchQ, 5)
	contacts := make([]omniContact, 0, len(matches))
	for _, c := range matches {
		contacts = append(contacts, omniContact{
			ID:        c.ID,
			Name:      c.Name,
			Company:   c.Company,
			UpdatedAt: c.UpdatedAt,
		})
	}

	var opts []tenantdb.Contact
	if looksLikeNote(q) {
		// If we can't infer a target from the note text, still offer one-shot logging
		// against a few common contacts so the palette isn't empty.
		if o, err := db.ContactOptions(); err == nil && len(o) > 0 {
			opts = o
		}
	}
	actions := omniBuildActions(time.Now(), q, matches, opts)

	// v2: prefer a flat row list with explicit kinds.
	rows := make([]map[string]any, 0, len(contacts)+len(actions))
	for _, c := range contacts {
		rows = append(rows, map[string]any{
			"kind":       "contact",
			"id":         c.ID,
			"name":       c.Name,
			"company":    c.Company,
			"updated_at": c.UpdatedAt,
		})
	}
	for _, a := range actions {
		row := map[string]any{"kind": a.Type}
		if a.ContactID != 0 {
			row["contact_id"] = a.ContactID
		}
		if a.ContactName != "" {
			row["contact_name"] = a.ContactName
		}
		if a.InteractionType != "" {
			row["interaction_type"] = a.InteractionType
		}
		if a.Content != "" {
			row["content"] = a.Content
		}
		if a.DueAt != "" {
			row["due_at"] = a.DueAt
		}
		if a.Name != "" {
			row["name"] = a.Name
		}
		rows = append(rows, row)
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"version":  2,
		"open":     true,
		"query":    q,
		"rows":     rows,
		"contacts": contacts,
		"actions":  actions,
	})
}

func (s *Server) handleQuickCreateContact(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	wantsJSON := strings.Contains(strings.ToLower(r.Header.Get("Accept")), "application/json")
	if !s.requireCSRF(w, r) {
		return
	}
	if err := parseMaybeMultipartForm(r); err != nil {
		if wantsJSON {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid form"})
			return
		}
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		if wantsJSON {
			s.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "contact name is required"})
			return
		}
		s.handleApp(w, r, tenant, appViewState{Flash: "Contact name is required."})
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if err := db.CreateContact(name, "", "", "", ""); err != nil {
		if wantsJSON {
			s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "contact creation failed"})
			return
		}
		s.handleApp(w, r, tenant, appViewState{Flash: "Contact creation failed: " + err.Error()})
		return
	}
	createdMatches, _ := db.SearchContacts(name, 1)
	if len(createdMatches) == 1 {
		if wantsJSON {
			c := createdMatches[0]
			s.writeJSON(w, http.StatusOK, map[string]any{
				"ok": true,
				"contact": omniContact{
					ID:        c.ID,
					Name:      c.Name,
					Company:   c.Company,
					UpdatedAt: c.UpdatedAt,
				},
			})
			return
		}
		http.Redirect(w, r, "/t/"+tenant.Slug+"/contacts/"+strconv.FormatInt(createdMatches[0].ID, 10), http.StatusSeeOther)
		return
	}
	if wantsJSON {
		s.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "contact creation failed"})
		return
	}
	s.handleApp(w, r, tenant, appViewState{Flash: "Contact created."})
}

func (s *Server) handleQuickCreateDeal(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	if err := parseMaybeMultipartForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	title := strings.TrimSpace(r.FormValue("title"))
	if title == "" {
		s.handleApp(w, r, tenant, appViewState{Flash: "Deal title is required."})
		return
	}
	contactID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("contact_id")), 10, 64)
	if err != nil || contactID <= 0 {
		s.handleApp(w, r, tenant, appViewState{Flash: "Deal creation failed: contact is required."})
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if _, err := db.ContactByID(contactID); err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Deal creation failed: contact does not exist."})
		return
	}
	dealID, err := db.CreateDeal(title, []int64{contactID})
	if err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Deal creation failed: " + err.Error()})
		return
	}
	_ = db.CreateDealEvent(dealID, "system", "Created from omnibar.")

	http.Redirect(w, r, "/t/"+tenant.Slug+"/deals/"+strconv.FormatInt(dealID, 10), http.StatusSeeOther)
}
