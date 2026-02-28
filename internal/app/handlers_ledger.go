package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleLedgerTimeline(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}

	q := r.URL.Query()
	actor := strings.TrimSpace(q.Get("actor"))
	op := strings.TrimSpace(q.Get("op"))
	entityType := strings.TrimSpace(q.Get("entity"))

	var entityID *int64
	if raw := strings.TrimSpace(q.Get("id")); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			entityID = &n
		}
	}

	limit := 200
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 2000 {
			limit = n
		}
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	events, err := db.ListLedgerEventsFiltered(tenantdb.LedgerEventFilter{
		ActorKind:  actor,
		Op:         op,
		EntityType: entityType,
		EntityID:   entityID,
		Limit:      limit,
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	header := renderTopNavHeader(tenant)
	body := renderLedgerTimelineBody(tenant, events, ledgerFilterState{
		ActorKind:  actor,
		Op:         op,
		EntityType: entityType,
		EntityID:   entityID,
		Limit:      limit,
	})
	csrf, err := s.ensureCSRFCookie(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{
		Title:     "Ledger",
		Header:    header,
		MainID:    "main-content",
		MainClass: "max-w-5xl mx-auto px-4 py-6 lg:px-6",
		Body:      body,
		CSRFToken: csrf,
	})
}

