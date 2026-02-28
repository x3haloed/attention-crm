package app

import (
	"attention-crm/internal/control"
	"attention-crm/web"
	"errors"
	"io/fs"
	"net/http"
	"strings"
)

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(web.StaticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /setup", s.handleSetupForm)
	mux.HandleFunc("POST /setup/passkey/start", s.handleSetupPasskeyStart)
	mux.HandleFunc("POST /setup/passkey/finish", s.handleSetupPasskeyFinish)
	mux.HandleFunc("GET /t/", s.handleTenantRoute)
	mux.HandleFunc("POST /t/", s.handleTenantRoute)
	return s.requestIDMiddleware(s.loggingMiddleware(s.securityHeadersMiddleware(s.bodyLimitMiddleware(mux))))
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	count, err := s.control.TenantCount()
	if err != nil {
		s.internalError(w, r, err)
		return
	}

	session, _ := s.readSession(r)
	if session.TenantSlug != "" {
		http.Redirect(w, r, "/t/"+session.TenantSlug+"/app", http.StatusSeeOther)
		return
	}

	_ = s.rootTmpl.Execute(w, map[string]any{
		"TenantCount": count,
	})
}

func (s *Server) handleTenantRoute(w http.ResponseWriter, r *http.Request) {
	slug, rest, ok := parseTenantPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	tenant, err := s.control.TenantBySlug(slug)
	if err != nil {
		if errors.Is(err, control.ErrTenantNotFound) {
			http.NotFound(w, r)
			return
		}
		s.internalError(w, r, err)
		return
	}

	switch {
	case r.Method == http.MethodGet && rest == "/login":
		s.renderLogin(w, slug, "")
	case r.Method == http.MethodPost && rest == "/login/passkey/start":
		s.handleLoginPasskeyStart(w, r, tenant)
	case r.Method == http.MethodPost && rest == "/login/passkey/finish":
		s.handleLoginPasskeyFinish(w, r, tenant)
	case r.Method == http.MethodPost && rest == "/login/passkey/discoverable/start":
		s.handleLoginPasskeyDiscoverableStart(w, r, tenant)
	case r.Method == http.MethodPost && rest == "/login/passkey/discoverable/finish":
		s.handleLoginPasskeyDiscoverableFinish(w, r, tenant)
	case r.Method == http.MethodPost && rest == "/logout":
		s.handleLogout(w, r, tenant)
	case r.Method == http.MethodGet && rest == "/omni":
		s.handleOmni(w, r, tenant)
	case r.Method == http.MethodGet && rest == "/app":
		s.handleApp(w, r, tenant, appViewState{})
	case r.Method == http.MethodGet && rest == "/export":
		s.handleExportPage(w, r, tenant)
	case r.Method == http.MethodGet && rest == "/export/contacts.csv":
		s.handleExportContactsCSV(w, r, tenant)
	case r.Method == http.MethodGet && rest == "/export/interactions.csv":
		s.handleExportInteractionsCSV(w, r, tenant)
	case r.Method == http.MethodGet && rest == "/export/deals.csv":
		s.handleExportDealsCSV(w, r, tenant)
	case r.Method == http.MethodGet && rest == "/ledger":
		s.handleLedgerTimeline(w, r, tenant)
	case r.Method == http.MethodPost && rest == "/contacts":
		s.handleCreateContact(w, r, tenant)
	case r.Method == http.MethodPost && rest == "/contacts/quick":
		s.handleQuickCreateContact(w, r, tenant)
	case r.Method == http.MethodPost && rest == "/deals/quick":
		s.handleQuickCreateDeal(w, r, tenant)
	case r.Method == http.MethodGet && rest == "/deals":
		s.handleDealsPipeline(w, r, tenant)
	case r.Method == http.MethodGet && strings.HasPrefix(rest, "/contacts/"):
		s.handleContactDetail(w, r, tenant, rest)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/contacts/") && strings.HasSuffix(rest, "/update"):
		s.handleUpdateContact(w, r, tenant, rest)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/contacts/") && strings.HasSuffix(rest, "/interactions"):
		s.handleCreateInteractionFromContact(w, r, tenant, rest)
	case r.Method == http.MethodGet && strings.HasPrefix(rest, "/deals/"):
		s.handleDealDesk(w, r, tenant, rest)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/deals/") && strings.HasSuffix(rest, "/next-step"):
		s.handleDealUpdateNextStep(w, r, tenant, rest)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/deals/") && strings.HasSuffix(rest, "/next-step/complete"):
		s.handleDealCompleteNextStep(w, r, tenant, rest)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/deals/") && strings.HasSuffix(rest, "/events"):
		s.handleDealCreateEvent(w, r, tenant, rest)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/deals/") && strings.HasSuffix(rest, "/close"):
		s.handleDealClose(w, r, tenant, rest)
	case r.Method == http.MethodPost && rest == "/interactions":
		s.handleCreateInteraction(w, r, tenant)
	case r.Method == http.MethodPost && rest == "/interactions/quick":
		s.handleQuickCreateInteraction(w, r, tenant)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/interactions/") && strings.HasSuffix(rest, "/complete"):
		s.handleCompleteInteraction(w, r, tenant, rest)
	case r.Method == http.MethodPost && rest == "/universal":
		s.handleUniversalInput(w, r, tenant)
	case r.Method == http.MethodPost && rest == "/invites":
		s.handleCreateInvite(w, r, tenant)
	case r.Method == http.MethodGet && rest == "/members":
		s.handleMembersPage(w, r, tenant)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/invites/") && strings.HasSuffix(rest, "/revoke"):
		s.handleRevokeInvite(w, r, tenant, rest)
	case r.Method == http.MethodGet && strings.HasPrefix(rest, "/invite/"):
		s.handleInvitePage(w, r, tenant, rest)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/invite/") && strings.HasSuffix(rest, "/passkey/start"):
		s.handleInvitePasskeyStart(w, r, tenant, rest)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/invite/") && strings.HasSuffix(rest, "/passkey/finish"):
		s.handleInvitePasskeyFinish(w, r, tenant, rest)
	default:
		http.NotFound(w, r)
	}
}
