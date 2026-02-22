package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"encoding/json"
	"net/http"
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

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	dups, err := db.DuplicateCandidates(name, email, phone, company, 10)
	if err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Duplicate check failed: " + err.Error()})
		return
	}

	err = db.CreateContact(name, email, phone, company, notes)
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
	s.handleContactDetailWithFlash(w, r, tenant, contactID, "")
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

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	updatedAt, err := db.UpdateContactField(contactID, payload.Field, payload.Value)
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

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	header := renderContactHeader(tenant, contact)
	body := renderContactDetailBody(tenant, contact, timeline, flash)
	csrf := s.ensureCSRFCookie(w, r)
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{
		Title:     contact.Name,
		Header:    header,
		MainID:    "main-content",
		MainClass: "max-w-4xl mx-auto px-4 py-6 lg:px-6",
		Body:      body,
		CSRFToken: csrf,
	})
}
