package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"net/http"
)

func (s *Server) handleDealsPipeline(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	rows, err := db.ListDealsPipeline(200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	header := renderDealsPipelineHeader(tenant)
	body := renderDealsPipelineBody(tenant, rows)
	csrf := s.ensureCSRFCookie(w, r)
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{
		Title:     "Deals",
		Header:    header,
		MainID:    "main-content",
		MainClass: "max-w-4xl mx-auto px-4 py-6 lg:px-6",
		Body:      body,
		CSRFToken: csrf,
	})
}
