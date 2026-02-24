package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"fmt"
	"net/http"
	"time"
)

func (s *Server) handleExportPage(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}

	body := renderExportBody(tenant)
	csrf := s.ensureCSRFCookie(w, r)
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{
		Title:     "Export",
		OmniBar:   renderOmniBar(tenant, "", "header"),
		Body:      body,
		CSRFToken: csrf,
	})
}

func (s *Server) handleExportContactsCSV(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	s.exportCSV(w, r, tenant, "contacts", func(w http.ResponseWriter, db *tenantdb.Store) error {
		return db.WriteContactsCSV(w)
	})
}

func (s *Server) handleExportInteractionsCSV(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	s.exportCSV(w, r, tenant, "interactions", func(w http.ResponseWriter, db *tenantdb.Store) error {
		return db.WriteInteractionsCSV(w)
	})
}

func (s *Server) handleExportDealsCSV(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	s.exportCSV(w, r, tenant, "deals", func(w http.ResponseWriter, db *tenantdb.Store) error {
		return db.WriteDealsCSV(w)
	})
}

func (s *Server) exportCSV(
	w http.ResponseWriter,
	r *http.Request,
	tenant control.Tenant,
	kind string,
	write func(w http.ResponseWriter, db *tenantdb.Store) error,
) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
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

	date := time.Now().UTC().Format("2006-01-02")
	filename := fmt.Sprintf("attention-crm-%s-%s-%s.csv", tenant.Slug, kind, date)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")

	if err := write(w, db); err != nil {
		s.internalError(w, r, err)
		return
	}
}
