package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"net/http"
)

func (s *Server) handleApp(w http.ResponseWriter, r *http.Request, tenant control.Tenant, state appViewState) {
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

	contacts, err := db.ContactOptions()
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	needsAttention, err := db.ListNeedsAttention(50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	needsDeals, err := db.ListDealsNeedsAttention(20)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	recent, err := db.ListRecentInteractions(50)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	agentPast, err := db.ListRecentNonCurrentActivityEventsByActorKind(tenantdb.ActorKindAgent, 8)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	agentCurrent, err := db.CurrentActivityEventByActorKind(tenantdb.ActorKindAgent)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	body := renderTenantAppBody(tenant, sess.UserID, state, contacts, needsAttention, needsDeals, recent, agentPast, agentCurrent)
	csrf, err := s.ensureCSRFCookie(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{Title: "Attention CRM", Body: body, CSRFToken: csrf})
}
