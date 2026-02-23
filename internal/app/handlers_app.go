package app

import (
	"attention-crm/internal/control"
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	contacts, err := db.ContactOptions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	needsAttention, err := db.ListNeedsAttention(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	needsDeals, err := db.ListDealsNeedsAttention(20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recent, err := db.ListRecentInteractions(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body := renderTenantAppBody(tenant, sess.UserID, state, contacts, needsAttention, needsDeals, recent)
	csrf := s.ensureCSRFCookie(w, r)
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{Title: "Attention CRM", Body: body, CSRFToken: csrf})
}
