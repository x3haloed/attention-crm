package app

import (
	"attention-crm/internal/control"
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

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	rows, err := db.ListDealsPipeline(200)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	header := renderDealsPipelineHeader(tenant)
	body := renderDealsPipelineBody(tenant, rows)
	csrf, err := s.ensureCSRFCookie(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{
		Title:     "Deals",
		Header:    header,
		MainID:    "main-content",
		MainClass: "max-w-4xl mx-auto px-4 py-6 lg:px-6",
		Body:      body,
		CSRFToken: csrf,
	})
}
