package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleMembersPage(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
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

	members, err := db.ListMembers(200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	invites, err := db.ListInvites(200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body := renderMembersBody(tenant, sess.UserID, members, invites)
	csrf := s.ensureCSRFCookie(w, r)
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{
		Title:     "Members",
		OmniBar:   renderOmniBar(tenant, "", "header"),
		Body:      body,
		CSRFToken: csrf,
	})
}

func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	inviteID, ok := parseInviteIDFromRevokeRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if err := db.RevokeInvite(inviteID, sess.UserID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/t/"+tenant.Slug+"/members", http.StatusSeeOther)
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
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
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	if email == "" {
		s.handleApp(w, r, tenant, appViewState{Flash: "Invite email is required."})
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	token, err := db.CreateInvite(sess.UserID, email, 7*24*time.Hour)
	if err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Invite creation failed: " + err.Error()})
		return
	}
	link := "/t/" + tenant.Slug + "/invite/" + token
	s.handleApp(w, r, tenant, appViewState{Flash: "Invite created. Copy the link below.", InviteLink: link})
}
