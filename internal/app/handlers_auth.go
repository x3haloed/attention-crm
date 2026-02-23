package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"errors"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}

	// Best-effort local logout: clear the session cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     "attention_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isSecureRequest(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "attention_csrf",
		Value:    "",
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isSecureRequest(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
}

func (s *Server) renderLogin(w http.ResponseWriter, slug, errText string) {
	_ = s.tenantAuth.ExecuteTemplate(w, "page", pageData{Title: "Login", Body: loginFormHTML(slug, errText)})
}

func (s *Server) handleLoginPasskeyStart(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	if !s.allowRate(r, "login_passkey_start|"+tenant.Slug, 1, 10) {
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
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	if email == "" {
		http.Error(w, "email required", http.StatusBadRequest)
		return
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	user, err := db.WebAuthnUserByEmail(email)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if len(user.Credentials) == 0 {
		http.Error(w, "no passkey enrolled for user", http.StatusBadRequest)
		return
	}

	options, sessionData, err := s.webauthn.BeginLogin(user, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		http.Error(w, "could not start login: "+err.Error(), http.StatusInternalServerError)
		return
	}
	flowID := s.storeFlow(ceremonyFlow{
		TenantSlug: tenant.Slug,
		UserID:     user.ID,
		Session:    *sessionData,
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})
	s.writeJSON(w, http.StatusOK, map[string]any{
		"flow_id": flowID,
		"options": options,
	})
}

func parseWebAuthnUserHandle(userHandle []byte) (int64, bool) {
	s := string(userHandle)
	if !strings.HasPrefix(s, "user:") {
		return 0, false
	}
	idRaw := strings.TrimSpace(strings.TrimPrefix(s, "user:"))
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (s *Server) handleLoginPasskeyDiscoverableStart(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	if !s.allowRate(r, "login_passkey_discoverable_start|"+tenant.Slug, 1, 10) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	if !requireSameOrigin(w, r) {
		return
	}

	options, sessionData, err := s.webauthn.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		http.Error(w, "could not start login: "+err.Error(), http.StatusInternalServerError)
		return
	}
	flowID := s.storeFlow(ceremonyFlow{
		TenantSlug: tenant.Slug,
		UserID:     0,
		Session:    *sessionData,
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
	})
	s.writeJSON(w, http.StatusOK, map[string]any{
		"flow_id": flowID,
		"options": options,
	})
}

func (s *Server) handleLoginPasskeyDiscoverableFinish(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	if !s.allowRate(r, "login_passkey_discoverable_finish|"+tenant.Slug, 1, 10) {
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
	if !ok || flow.TenantSlug != tenant.Slug {
		http.Error(w, "invalid or expired flow", http.StatusBadRequest)
		return
	}

	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		userID, ok := parseWebAuthnUserHandle(userHandle)
		if !ok {
			return nil, errors.New("invalid user handle")
		}
		u, err := db.WebAuthnUserByID(userID)
		if err != nil {
			return nil, err
		}
		if len(u.Credentials) == 0 {
			return nil, errors.New("no passkey enrolled for user")
		}
		return u, nil
	}

	u, _, err := s.webauthn.FinishPasskeyLogin(handler, flow.Session, r)
	if err != nil {
		http.Error(w, "passkey assertion failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	typed, ok := u.(tenantdb.WebAuthnUser)
	if !ok {
		http.Error(w, "could not resolve user", http.StatusInternalServerError)
		return
	}

	if err := s.writeSession(w, r, session{TenantSlug: tenant.Slug, UserID: typed.ID}); err != nil {
		http.Error(w, "set session failed", http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"redirect": "/t/" + tenant.Slug + "/app",
	})
}

func (s *Server) handleLoginPasskeyFinish(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	if !s.allowRate(r, "login_passkey_finish|"+tenant.Slug, 1, 10) {
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
	if !ok || flow.TenantSlug != tenant.Slug {
		http.Error(w, "invalid or expired flow", http.StatusBadRequest)
		return
	}

	db, err := s.openTenantDB(tenant.DBPath)
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
	if _, err := s.webauthn.FinishLogin(user, flow.Session, r); err != nil {
		http.Error(w, "passkey assertion failed: "+err.Error(), http.StatusBadRequest)
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

func (s *Server) handleInvitePage(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	token, ok := parseInviteToken(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	db, err := s.openTenantDB(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	if err := db.CompleteInviteRedemption(token, user.ID); err != nil {
		http.Error(w, "invite completion failed: "+err.Error(), http.StatusBadRequest)
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
