package app

import (
	"attention-crm/internal/control"
	"encoding/json"
	"net/http"
)

func (s *Server) handleShadowRope(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
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

	marker, items, _, err := s.shadowRopeSnapshot(db, tenant, sess, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"marker": marker,
		"items":  items,
	})
}
