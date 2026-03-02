package app

import (
	"attention-crm/internal/control"
	"errors"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"html/template"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleInvitePage(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	token, ok := parseInviteToken(rest)
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
	inv, err := db.InviteByToken(token)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if inv.RedeemedAt.Valid {
		_ = s.tenantAuth.ExecuteTemplate(w, "page", pageData{Title: "Invite", Body: template.HTML("<p>This invite link was already used.</p>")})
		return
	}

	body := inviteRedeemHTML(tenant.Slug, inv.Email, token)
	_ = s.tenantAuth.ExecuteTemplate(w, "page", pageData{Title: "Join Workspace", Body: body})
}

func (s *Server) handleInvitePasskeyStart(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	if !s.allowRate(r, "invite_passkey_start|"+tenant.Slug, 0.5, 10) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	if !requireSameOrigin(w, r) {
		return
	}
	token, ok := parseInviteToken(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := parseMaybeMultipartForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	defer db.Close()

	user, err := db.StartInviteRedemption(token, name)
	if err != nil {
		http.Error(w, "invite redeem failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	waUser, err := db.WebAuthnUserByID(user.ID)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	options, sessionData, err := s.webauthn.BeginRegistration(waUser, webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
		ResidentKey:      protocol.ResidentKeyRequirementPreferred,
		UserVerification: protocol.VerificationRequired,
	}))
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	flowID, err := s.storeFlow(ceremonyFlow{
		TenantSlug: tenant.Slug,
		UserID:     user.ID,
		Session:    *sessionData,
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"flow_id": flowID,
		"options": options,
	})
}

func (s *Server) handleInvitePasskeyFinish(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	if !s.allowRate(r, "invite_passkey_finish|"+tenant.Slug, 0.5, 10) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	if !requireSameOrigin(w, r) {
		return
	}
	token, ok := parseInviteToken(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}

	flowID := strings.TrimSpace(r.URL.Query().Get("flow_id"))
	if flowID == "" {
		http.Error(w, "missing flow_id", http.StatusBadRequest)
		return
	}
	flow, ok := s.consumeFlow(flowID)
	if !ok || flow.TenantSlug != tenant.Slug {
		http.Error(w, "invalid or expired flow", http.StatusBadRequest)
		return
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		s.internalError(w, r, err)
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
		s.internalError(w, r, errors.New("credential save failed"))
		return
	}
	if err := db.CompleteInviteRedemption(token, user.ID); err != nil {
		http.Error(w, "invite completion failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.writeSession(w, r, session{TenantSlug: tenant.Slug, UserID: user.ID}); err != nil {
		s.internalError(w, r, errors.New("set session failed"))
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"redirect": "/t/" + tenant.Slug + "/app",
	})
}
