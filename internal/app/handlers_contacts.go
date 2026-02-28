package app

import (
	"attention-crm/internal/control"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleCreateContact(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
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

	name := r.FormValue("name")
	email := r.FormValue("email")
	phone := r.FormValue("phone")
	company := r.FormValue("company")
	notes := r.FormValue("notes")
	if strings.TrimSpace(name) == "" {
		s.handleApp(w, r, tenant, appViewState{Flash: "Contact creation failed: contact name is required."})
		return
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	dups, err := db.DuplicateCandidates(name, email, phone, company, 10)
	if err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Duplicate check failed: " + err.Error()})
		return
	}

	_, err = db.CreateContactBy(sess.UserID, name, email, phone, company, notes)
	if err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Contact creation failed: " + err.Error(), Duplicates: dups})
		return
	}
	s.handleApp(w, r, tenant, appViewState{Flash: "Contact created.", Duplicates: dups})
}

func (s *Server) handleContactDetail(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	contactID, ok := parseContactIDFromRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	flash := ""
	switch strings.TrimSpace(r.URL.Query().Get("flash")) {
	case "interaction_logged":
		flash = "Interaction logged."
	}
	s.handleContactDetailWithFlash(w, r, tenant, contactID, flash)
}

func (s *Server) handleUpdateContact(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	contactID, ok := parseContactIDFromUpdateRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var payload struct {
		Field string `json:"field"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	updatedAt, err := db.UpdateContactFieldBy(sess.UserID, contactID, payload.Field, payload.Value)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"updated_at": updatedAt,
	})
}

func (s *Server) handleContactDetailWithFlash(w http.ResponseWriter, r *http.Request, tenant control.Tenant, contactID int64, flash string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	contact, err := db.ContactByID(contactID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	timeline, err := db.ListInteractionsByContact(contactID, 200)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	deals, err := db.ListDealsByContact(contactID, 50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	var highlightInteractionID int64
	if raw := strings.TrimSpace(r.URL.Query().Get("hl")); raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
			highlightInteractionID = id
		}
	}

	header := renderContactHeader(tenant, contact)
	body := renderContactDetailBody(tenant, contact, deals, timeline, flash, highlightInteractionID)
	csrf, err := s.ensureCSRFCookie(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	title := strings.TrimSpace(contact.Name)
	if title == "" {
		title = strings.TrimSpace(contact.Email)
	}
	if title == "" {
		title = strings.TrimSpace(contact.Phone)
	}
	if title == "" {
		title = "Contact"
	}

	s.renderTenantAppPage(w, r, tenant, db, pageData{
		Title:     title,
		TenantSlug: tenant.Slug,
		OmniBar:   renderOmniBar(tenant, "", "header"),
		MainID:    "main-content",
		Body:      template.HTML(`<div class="max-w-4xl mx-auto px-4 py-6 lg:px-6">` + string(header) + string(body) + `</div>`),
		CSRFToken: csrf,
	})
}
