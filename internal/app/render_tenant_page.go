package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"net/http"
	"strings"
)

func (s *Server) renderTenantAppPage(w http.ResponseWriter, r *http.Request, tenant control.Tenant, db *tenantdb.Store, pd pageData) {
	if strings.TrimSpace(pd.TenantSlug) == "" {
		pd.TenantSlug = tenant.Slug
	}
	if strings.TrimSpace(string(pd.OmniBar)) == "" {
		pd.OmniBar = renderOmniBar(tenant, "", "header")
	}
	if strings.TrimSpace(string(pd.Rail)) == "" && db != nil {
		rail, err := loadAgentRail(db)
		if err != nil {
			s.internalError(w, r, err)
			return
		}
		pd.Rail = rail
	}
	_ = s.tenantApp.ExecuteTemplate(w, "page", pd)
}
