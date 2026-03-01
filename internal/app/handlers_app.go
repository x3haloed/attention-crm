package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"encoding/json"
	"net/http"
	"strings"
)

type spineEvent struct {
	ActorKind string
	Title     string
	Summary   string
	DetailJSON string
	CreatedAt string
}

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

	body := renderTenantAppBody(tenant, sess.UserID, state, contacts, needsAttention, needsDeals, recent)
	csrf, err := s.ensureCSRFCookie(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.renderTenantAppPage(w, r, tenant, db, pageData{
		Title:      "Attention CRM",
		TenantSlug: tenant.Slug,
		OmniBar:    renderOmniBar(tenant, state.UniversalText, "header"),
		Body:       body,
		CSRFToken:  csrf,
	})
}

type spinePayload struct {
	Status     string `json:"status"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	DetailJSON string `json:"detail_json"`
}

func splitAgentSpineEvents(events []tenantdb.LedgerEvent) ([]spineEvent, *spineEvent) {
	var past []spineEvent
	var current *spineEvent

	for _, ev := range events {
		raw := strings.TrimSpace(ev.PayloadJSON)
		if raw == "" {
			continue
		}
		var payload spinePayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			continue
		}

		title := strings.TrimSpace(payload.Title)
		if title == "" {
			continue
		}

		out := spineEvent{
			ActorKind:  ev.ActorKind,
			Title:      title,
			Summary:    strings.TrimSpace(payload.Summary),
			DetailJSON: strings.TrimSpace(payload.DetailJSON),
			CreatedAt:  ev.CreatedAt,
		}

		if current == nil && strings.EqualFold(strings.TrimSpace(payload.Status), "current") {
			tmp := out
			current = &tmp
			continue
		}
		past = append(past, out)
	}

	return past, current
}
