package app

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
)

func (s *Server) handleDealDesk(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	dealID, ok := parseDealIDFromRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	deal, contactIDs, err := db.DealByID(dealID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	events, err := db.ListDealEvents(dealID, 200)
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	var targets []tenantdb.Contact
	for _, cid := range contactIDs {
		c, err := db.ContactByID(cid)
		if err == nil {
			targets = append(targets, c)
		}
	}

	body := renderDealDeskBody(tenant, deal, targets, events, "")
	csrf, err := s.ensureCSRFCookie(w, r)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{
		Title:     "Deal",
		OmniBar:   renderOmniBar(tenant, "", "header"),
		MainID:    "main-content",
		MainClass: "max-w-4xl mx-auto px-4 py-6 lg:px-6",
		Body:      body,
		CSRFToken: csrf,
	})
}

func (s *Server) handleDealUpdateNextStep(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	dealID, ok := parseDealIDFromRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := parseMaybeMultipartForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	nextStep := strings.TrimSpace(r.FormValue("next_step"))
	dueAtRaw := strings.TrimSpace(r.FormValue("due_at"))

	var dueAt *time.Time
	if dueAtRaw != "" {
		parsed, parseErr := time.Parse("2006-01-02T15:04", dueAtRaw)
		if parseErr != nil {
			http.Redirect(w, r, "/t/"+tenant.Slug+"/deals/"+strconv.FormatInt(dealID, 10), http.StatusSeeOther)
			return
		}
		dueAt = &parsed
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	if _, err := db.UpdateDealNextStep(dealID, nextStep, dueAt); err != nil {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/deals/"+strconv.FormatInt(dealID, 10), http.StatusSeeOther)
		return
	}
	_ = db.CreateDealEventBy(sess.UserID, dealID, "system", "Updated next step.")
	http.Redirect(w, r, "/t/"+tenant.Slug+"/deals/"+strconv.FormatInt(dealID, 10), http.StatusSeeOther)
}

func (s *Server) handleDealCompleteNextStep(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	dealID, ok := parseDealIDFromRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()
	_, _ = db.CompleteDealNextStep(dealID)
	_ = db.CreateDealEventBy(sess.UserID, dealID, "system", "Completed next step.")
	http.Redirect(w, r, "/t/"+tenant.Slug+"/deals/"+strconv.FormatInt(dealID, 10), http.StatusSeeOther)
}

func (s *Server) handleDealCreateEvent(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	dealID, ok := parseDealIDFromRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := parseMaybeMultipartForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	typ := strings.TrimSpace(r.FormValue("type"))
	content := strings.TrimSpace(r.FormValue("content"))
	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	_ = db.CreateDealEventBy(sess.UserID, dealID, typ, content)
	http.Redirect(w, r, "/t/"+tenant.Slug+"/deals/"+strconv.FormatInt(dealID, 10), http.StatusSeeOther)
}

func (s *Server) handleDealClose(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	dealID, ok := parseDealIDFromRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := parseMaybeMultipartForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	state := strings.TrimSpace(r.FormValue("state"))
	outcome := strings.TrimSpace(r.FormValue("outcome"))

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	if _, err := db.CloseDeal(dealID, state, outcome); err == nil {
		_ = db.CreateDealEventBy(sess.UserID, dealID, "system", "Closed "+strings.ToUpper(state)+": "+outcome)
	}
	http.Redirect(w, r, "/t/"+tenant.Slug+"/deals/"+strconv.FormatInt(dealID, 10), http.StatusSeeOther)
}
