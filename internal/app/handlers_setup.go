package app

import (
	"attention-crm/internal/tenantdb"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleSetupForm(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/setup" {
		http.NotFound(w, r)
		return
	}
	count, err := s.control.TenantCount()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	_ = s.tenantAuth.ExecuteTemplate(w, "page", pageData{Title: "Initial Setup", Body: setupFormHTML("", "")})
}

func (s *Server) handleSetupPasskeyStart(w http.ResponseWriter, r *http.Request) {
	if !s.allowRate(r, "setup_passkey_start", 0.5, 5) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	if !requireSameOrigin(w, r) {
		return
	}
	if err := parseMaybeMultipartForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	count, err := s.control.TenantCount()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Error(w, "setup already completed", http.StatusConflict)
		return
	}

	workspaceName := strings.TrimSpace(r.FormValue("workspace_name"))
	tenantSlug := strings.TrimSpace(r.FormValue("tenant_slug"))
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	name := strings.TrimSpace(r.FormValue("name"))
	if workspaceName == "" || tenantSlug == "" || email == "" || name == "" {
		http.Error(w, "workspace, slug, email, and name are required", http.StatusBadRequest)
		return
	}

	tenant, err := s.control.CreateTenant(tenantSlug, workspaceName)
	if err != nil {
		http.Error(w, "could not create tenant", http.StatusBadRequest)
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	user, err := db.CreateInitialUserForPasskey(workspaceName, email, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	waUser, err := db.WebAuthnUserByID(user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	options, sessionData, err := s.webauthn.BeginRegistration(waUser, webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
		ResidentKey:      protocol.ResidentKeyRequirementPreferred,
		UserVerification: protocol.VerificationRequired,
	}))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	flowID := s.storeFlow(ceremonyFlow{
		TenantSlug: tenant.Slug,
		UserID:     user.ID,
		Session:    *sessionData,
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})

	s.writeJSON(w, http.StatusOK, map[string]any{
		"flow_id":     flowID,
		"tenant_slug": tenant.Slug,
		"options":     options,
	})
}

func (s *Server) handleSetupPasskeyFinish(w http.ResponseWriter, r *http.Request) {
	if !s.allowRate(r, "setup_passkey_finish", 0.5, 5) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	if !requireSameOrigin(w, r) {
		return
	}
	flowID := strings.TrimSpace(r.URL.Query().Get("flow_id"))
	if flowID == "" {
		http.Error(w, "missing flow_id", http.StatusBadRequest)
		return
	}
	flow, ok := s.consumeFlow(flowID)
	if !ok {
		http.Error(w, "invalid or expired flow", http.StatusBadRequest)
		return
	}

	tenant, err := s.control.TenantBySlug(flow.TenantSlug)
	if err != nil {
		http.Error(w, "tenant not found", http.StatusBadRequest)
		return
	}
	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	user, err := db.WebAuthnUserByID(flow.UserID)
	if err != nil {
		http.Error(w, "user not found", http.StatusBadRequest)
		return
	}

	credential, err := s.webauthn.FinishRegistration(user, flow.Session, r)
	if err != nil {
		http.Error(w, "passkey registration failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := db.AddWebAuthnCredential(user.ID, credential); err != nil {
		http.Error(w, "credential save failed", http.StatusInternalServerError)
		return
	}
	if err := s.writeSession(w, r, session{TenantSlug: tenant.Slug, UserID: user.ID}); err != nil {
		http.Error(w, "set session failed", http.StatusInternalServerError)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"redirect": "/t/" + tenant.Slug + "/app",
	})
}
