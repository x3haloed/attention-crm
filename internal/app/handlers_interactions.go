package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleQuickCreateInteraction(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
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

	contactID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("contact_id")), 10, 64)
	if err != nil || contactID <= 0 {
		s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: contact is required."})
		return
	}
	interactionType := strings.TrimSpace(r.FormValue("type"))
	if interactionType == "" {
		interactionType = "note"
	}
	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" {
		s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: content is required."})
		return
	}

	dueAtRaw := strings.TrimSpace(r.FormValue("due_at"))
	var dueAt *time.Time
	if dueAtRaw != "" {
		parsed, parseErr := time.Parse("2006-01-02T15:04", dueAtRaw)
		if parseErr != nil {
			s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: due date format is invalid."})
			return
		}
		dueAt = &parsed
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if _, err := db.ContactByID(contactID); err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: contact does not exist."})
		return
	}
	if err := db.CreateInteraction(contactID, interactionType, content, dueAt); err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: " + err.Error()})
		return
	}

	// Redirect to the contact and explicitly highlight the newly-created interaction so users
	// don't feel like their omnibar entry was lost.
	interactionID, _ := db.LatestInteractionIDByContact(contactID)

	redirect := "/t/" + tenant.Slug + "/contacts/" + strconv.FormatInt(contactID, 10) + "?flash=interaction_logged"
	if interactionID > 0 {
		redirect = redirect + "&hl=" + strconv.FormatInt(interactionID, 10) + "#interaction-" + strconv.FormatInt(interactionID, 10)
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) handleCreateInteraction(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
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
	contactID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("contact_id")), 10, 64)
	if err != nil || contactID <= 0 {
		s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: contact is required."})
		return
	}
	interactionType := strings.TrimSpace(r.FormValue("type"))
	content := r.FormValue("content")
	dueAtRaw := strings.TrimSpace(r.FormValue("due_at"))

	var dueAt *time.Time
	if dueAtRaw != "" {
		parsed, parseErr := time.Parse("2006-01-02T15:04", dueAtRaw)
		if parseErr != nil {
			s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: due date format is invalid."})
			return
		}
		dueAt = &parsed
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if _, err := db.ContactByID(contactID); err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: contact does not exist."})
		return
	}
	if err := db.CreateInteraction(contactID, interactionType, content, dueAt); err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: " + err.Error()})
		return
	}
	s.handleApp(w, r, tenant, appViewState{Flash: "Interaction logged."})
}

func (s *Server) handleCreateInteractionFromContact(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	contactID, ok := parseContactIDFromRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	interactionType := strings.TrimSpace(r.FormValue("type"))
	content := r.FormValue("content")
	dueAtRaw := strings.TrimSpace(r.FormValue("due_at"))
	var dueAt *time.Time
	if dueAtRaw != "" {
		parsed, parseErr := time.Parse("2006-01-02T15:04", dueAtRaw)
		if parseErr != nil {
			s.handleContactDetailWithFlash(w, r, tenant, contactID, "Interaction creation failed: due date format is invalid.")
			return
		}
		dueAt = &parsed
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if _, err := db.ContactByID(contactID); err != nil {
		s.handleContactDetailWithFlash(w, r, tenant, contactID, "Contact not found.")
		return
	}
	if err := db.CreateInteraction(contactID, interactionType, content, dueAt); err != nil {
		s.handleContactDetailWithFlash(w, r, tenant, contactID, "Interaction creation failed: "+err.Error())
		return
	}
	interactionID, _ := db.LatestInteractionIDByContact(contactID)

	redirect := "/t/" + tenant.Slug + "/contacts/" + strconv.FormatInt(contactID, 10) + "?flash=interaction_logged"
	if interactionID > 0 {
		redirect = redirect + "&hl=" + strconv.FormatInt(interactionID, 10) + "#interaction-" + strconv.FormatInt(interactionID, 10)
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (s *Server) handleCompleteInteraction(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	trimmed := strings.TrimPrefix(rest, "/interactions/")
	interactionIDRaw := strings.TrimSuffix(trimmed, "/complete")
	interactionIDRaw = strings.Trim(interactionIDRaw, "/")
	interactionID, err := strconv.ParseInt(interactionIDRaw, 10, 64)
	if err != nil || interactionID <= 0 {
		s.handleApp(w, r, tenant, appViewState{Flash: "Could not complete interaction: invalid id."})
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if err := db.MarkInteractionComplete(interactionID); err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Could not complete interaction: " + err.Error()})
		return
	}
	s.handleApp(w, r, tenant, appViewState{Flash: "Interaction marked complete."})
}
