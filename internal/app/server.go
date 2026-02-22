package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"attention-crm/web"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"golang.org/x/time/rate"
)

type Server struct {
	cfg          Config
	control      *control.Store
	sessionKey   []byte
	rootTmpl     *template.Template
	tenantAuth   *template.Template
	tenantApp    *template.Template
	webauthn     *webauthn.WebAuthn
	flowMu       sync.Mutex
	webauthnFlow map[string]ceremonyFlow

	limMu    sync.Mutex
	lim      map[string]*rate.Limiter
	limSeen  map[string]time.Time
	limSweep time.Time
}

type ceremonyFlow struct {
	TenantSlug string
	UserID     int64
	Session    webauthn.SessionData
	ExpiresAt  time.Time
}

func NewServer(cfg Config) (*Server, error) {
	controlStore, err := control.Open(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	key, err := loadOrCreateSessionKey(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	rootTmpl := template.Must(template.New("root").Parse(rootPageTemplate))
	tenantBase := template.Must(template.New("tenant_base").Parse(tenantBaseTemplate))
	tenantAuth := template.Must(template.Must(tenantBase.Clone()).Parse(tenantAuthTemplate))
	tenantApp := template.Must(template.Must(tenantBase.Clone()).Parse(tenantAppTemplate))
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.WebAuthnRPID,
		RPDisplayName: cfg.WebAuthnName,
		RPOrigins:     cfg.WebAuthnOrigins,
	})
	if err != nil {
		return nil, err
	}

	return &Server{
		cfg:          cfg,
		control:      controlStore,
		sessionKey:   key,
		rootTmpl:     rootTmpl,
		tenantAuth:   tenantAuth,
		tenantApp:    tenantApp,
		webauthn:     wa,
		webauthnFlow: map[string]ceremonyFlow{},
		lim:          map[string]*rate.Limiter{},
		limSeen:      map[string]time.Time{},
	}, nil
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func (s *Server) allowRate(r *http.Request, bucket string, perSecond float64, burst int) bool {
	ip := clientIP(r)
	key := bucket + "|" + ip

	s.limMu.Lock()
	defer s.limMu.Unlock()

	now := time.Now()
	s.limSeen[key] = now

	lim := s.lim[key]
	if lim == nil {
		lim = rate.NewLimiter(rate.Limit(perSecond), burst)
		s.lim[key] = lim
	}

	// Opportunistic sweep to bound memory.
	if s.limSweep.IsZero() || now.Sub(s.limSweep) > 5*time.Minute {
		cutoff := now.Add(-15 * time.Minute)
		for k, seen := range s.limSeen {
			if seen.Before(cutoff) {
				delete(s.limSeen, k)
				delete(s.lim, k)
			}
		}
		s.limSweep = now
	}

	return lim.Allow()
}

func randomTokenB64(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}

func (s *Server) ensureCSRFCookie(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie("attention_csrf"); err == nil && c.Value != "" {
		return c.Value
	}
	token := randomTokenB64(32)
	http.SetCookie(w, &http.Cookie{
		Name:     "attention_csrf",
		Value:    token,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		Expires:  time.Now().Add(24 * time.Hour),
	})
	return token
}

func (s *Server) requireCSRF(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}

	c, err := r.Cookie("attention_csrf")
	if err != nil || c.Value == "" {
		http.Error(w, "missing csrf token", http.StatusForbidden)
		return false
	}

	token := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	if token == "" {
		// Allow non-JS form posts by validating same-origin (Origin/Referer).
		if sfs := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); sfs == "same-origin" || sfs == "same-site" {
			return true
		}
		ref := r.Header.Get("Origin")
		if ref == "" {
			ref = r.Header.Get("Referer")
		}
		u, parseErr := url.Parse(ref)
		if parseErr == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host) {
			return true
		}
		http.Error(w, "missing csrf header", http.StatusForbidden)
		return false
	}
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(c.Value)) != 1 {
		http.Error(w, "csrf token mismatch", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(web.StaticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("GET /setup", s.handleSetupForm)
	mux.HandleFunc("POST /setup/passkey/start", s.handleSetupPasskeyStart)
	mux.HandleFunc("POST /setup/passkey/finish", s.handleSetupPasskeyFinish)
	mux.HandleFunc("GET /t/", s.handleTenantRoute)
	mux.HandleFunc("POST /t/", s.handleTenantRoute)
	return loggingMiddleware(mux)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	count, err := s.control.TenantCount()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	case r.Method == http.MethodPost && rest == "/contacts":
		s.handleCreateContact(w, r, tenant)
	case r.Method == http.MethodPost && rest == "/contacts/quick":
		s.handleQuickCreateContact(w, r, tenant)
	case r.Method == http.MethodGet && strings.HasPrefix(rest, "/contacts/"):
		s.handleContactDetail(w, r, tenant, rest)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/contacts/") && strings.HasSuffix(rest, "/update"):
		s.handleUpdateContact(w, r, tenant, rest)
	case r.Method == http.MethodPost && strings.HasPrefix(rest, "/contacts/") && strings.HasSuffix(rest, "/interactions"):
		s.handleCreateInteractionFromContact(w, r, tenant, rest)
	case r.Method == http.MethodPost && rest == "/interactions":
		s.handleCreateInteraction(w, r, tenant)
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

func (s *Server) handleMembersPage(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	members, err := db.ListMembers(200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	invites, err := db.ListInvites(200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body := renderMembersBody(tenant, sess.UserID, members, invites)
	csrf := s.ensureCSRFCookie(w, r)
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{Title: "Members", Body: body, CSRFToken: csrf})
}

func parseInviteIDFromRevokeRest(rest string) (int64, bool) {
	trimmed := strings.TrimPrefix(rest, "/invites/")
	trimmed = strings.TrimSuffix(trimmed, "/revoke")
	trimmed = strings.Trim(trimmed, "/")
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	inviteID, ok := parseInviteIDFromRevokeRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if err := db.RevokeInvite(inviteID, sess.UserID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/t/"+tenant.Slug+"/members", http.StatusSeeOther)
}

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
		Secure:   isSecureRequest(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "attention_csrf",
		Value:    "",
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
	http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
}

type omniContact struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Company   string `json:"company"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Server) handleOmni(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		s.writeJSON(w, http.StatusOK, map[string]any{
			"open":     false,
			"query":    "",
			"contacts": []omniContact{},
			"actions":  []map[string]any{},
		})
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	searchQ := q
	if looksLikeNote(q) {
		if cq := extractContactQueryFromNote(q); cq != "" {
			searchQ = cq
		}
	}
	matches, _ := db.SearchContacts(searchQ, 5)
	contacts := make([]omniContact, 0, len(matches))
	for _, c := range matches {
		contacts = append(contacts, omniContact{
			ID:        c.ID,
			Name:      c.Name,
			Company:   c.Company,
			UpdatedAt: c.UpdatedAt,
		})
	}

	actions := []map[string]any{}
	if looksLikeContactName(q) {
		actions = append(actions, map[string]any{
			"type": "create_contact",
			"name": q,
		})
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"open":     true,
		"query":    q,
		"contacts": contacts,
		"actions":  actions,
	})
}

func (s *Server) handleQuickCreateContact(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	if err := parseMaybeMultipartForm(r); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		s.handleApp(w, r, tenant, appViewState{Flash: "Contact name is required."})
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if err := db.CreateContact(name, "", "", "", ""); err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Contact creation failed: " + err.Error()})
		return
	}
	createdMatches, _ := db.SearchContacts(name, 1)
	if len(createdMatches) == 1 {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/contacts/"+strconv.FormatInt(createdMatches[0].ID, 10), http.StatusSeeOther)
		return
	}
	s.handleApp(w, r, tenant, appViewState{Flash: "Contact created."})
}

func (s *Server) renderLogin(w http.ResponseWriter, slug, errText string) {
	_ = s.tenantAuth.ExecuteTemplate(w, "page", pageData{Title: "Login", Body: loginFormHTML(slug, errText)})
}

func (s *Server) handleLoginPasskeyStart(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	if !s.allowRate(r, "login_passkey_start|"+tenant.Slug, 1, 10) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
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

	db, err := tenantdb.Open(tenant.DBPath)
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

	db, err := tenantdb.Open(tenant.DBPath)
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

func (s *Server) handleApp(w http.ResponseWriter, r *http.Request, tenant control.Tenant, state appViewState) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	contacts, err := db.ContactOptions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	needsAttention, err := db.ListNeedsAttention(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	recent, err := db.ListRecentInteractions(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body := renderTenantAppBody(tenant, sess.UserID, state, contacts, needsAttention, recent)
	csrf := s.ensureCSRFCookie(w, r)
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{Title: "Attention CRM", Body: body, CSRFToken: csrf})
}

func (s *Server) handleCreateContact(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	email := r.FormValue("email")
	phone := r.FormValue("phone")
	company := r.FormValue("company")
	notes := r.FormValue("notes")

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	dups, err := db.DuplicateCandidates(name, email, phone, company, 10)
	if err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Duplicate check failed: " + err.Error()})
		return
	}

	err = db.CreateContact(name, email, phone, company, notes)
	if err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Contact creation failed: " + err.Error(), Duplicates: dups})
		return
	}
	s.handleApp(w, r, tenant, appViewState{Flash: "Contact created.", Duplicates: dups})
}

func (s *Server) handleCreateInteraction(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	contactID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("contact_id")), 10, 64)
	if err != nil || contactID <= 0 {
		s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: contact is required."})
		return
	}
	interactionType := strings.TrimSpace(r.FormValue("type"))
	content := r.FormValue("content")
	dueAtRaw := strings.TrimSpace(r.FormValue("due_at"))

	var dueAt *time.Time
	if dueAtRaw != "" {
		parsed, parseErr := time.Parse("2006-01-02T15:04", dueAtRaw)
		if parseErr != nil {
			s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: due date format is invalid."})
			return
		}
		dueAt = &parsed
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if _, err := db.ContactByID(contactID); err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: contact does not exist."})
		return
	}
	if err := db.CreateInteraction(contactID, interactionType, content, dueAt); err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Interaction creation failed: " + err.Error()})
		return
	}
	s.handleApp(w, r, tenant, appViewState{Flash: "Interaction logged."})
}

func (s *Server) handleCreateInteractionFromContact(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	contactID, ok := parseContactIDFromRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	interactionType := strings.TrimSpace(r.FormValue("type"))
	content := r.FormValue("content")
	dueAtRaw := strings.TrimSpace(r.FormValue("due_at"))
	var dueAt *time.Time
	if dueAtRaw != "" {
		parsed, parseErr := time.Parse("2006-01-02T15:04", dueAtRaw)
		if parseErr != nil {
			s.handleContactDetailWithFlash(w, r, tenant, contactID, "Interaction creation failed: due date format is invalid.")
			return
		}
		dueAt = &parsed
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if _, err := db.ContactByID(contactID); err != nil {
		s.handleContactDetailWithFlash(w, r, tenant, contactID, "Contact not found.")
		return
	}
	if err := db.CreateInteraction(contactID, interactionType, content, dueAt); err != nil {
		s.handleContactDetailWithFlash(w, r, tenant, contactID, "Interaction creation failed: "+err.Error())
		return
	}
	s.handleContactDetailWithFlash(w, r, tenant, contactID, "Interaction logged.")
}

func (s *Server) handleCompleteInteraction(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	trimmed := strings.TrimPrefix(rest, "/interactions/")
	interactionIDRaw := strings.TrimSuffix(trimmed, "/complete")
	interactionIDRaw = strings.Trim(interactionIDRaw, "/")
	interactionID, err := strconv.ParseInt(interactionIDRaw, 10, 64)
	if err != nil || interactionID <= 0 {
		s.handleApp(w, r, tenant, appViewState{Flash: "Could not complete interaction: invalid id."})
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if err := db.MarkInteractionComplete(interactionID); err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Could not complete interaction: " + err.Error()})
		return
	}
	s.handleApp(w, r, tenant, appViewState{Flash: "Interaction marked complete."})
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(strings.ToLower(r.FormValue("email")))
	if email == "" {
		s.handleApp(w, r, tenant, appViewState{Flash: "Invite email is required."})
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	token, err := db.CreateInvite(sess.UserID, email, 7*24*time.Hour)
	if err != nil {
		s.handleApp(w, r, tenant, appViewState{Flash: "Invite creation failed: " + err.Error()})
		return
	}
	link := "/t/" + tenant.Slug + "/invite/" + token
	s.handleApp(w, r, tenant, appViewState{Flash: "Invite created. Copy the link below.", InviteLink: link})
}

func (s *Server) handleUniversalInput(w http.ResponseWriter, r *http.Request, tenant control.Tenant) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	query := strings.TrimSpace(r.FormValue("q"))
	if query == "" {
		s.handleApp(w, r, tenant, appViewState{Flash: "Universal input is empty."})
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	if looksLikeNote(query) {
		contactQuery := extractContactQueryFromNote(query)
		hints := []tenantdb.Contact{}
		if contactQuery != "" {
			hints, _ = db.SearchContacts(contactQuery, 10)
		}
		if len(hints) == 0 {
			hints, _ = db.ContactOptions()
			if len(hints) > 30 {
				hints = hints[:30]
			}
		}
		dueLocal, _ := parseDueSuggestionLocal(query, time.Now())

		s.handleApp(w, r, tenant, appViewState{
			Flash:         "This looks like a note.",
			UniversalText: query,
			SearchResults: hints,
			SuggestNote: &noteSuggestion{
				Content:      query,
				DueAtLocal:   dueLocal,
				ContactHints: hints,
			},
		})
		return
	}

	matches, err := db.SearchContacts(query, 10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(matches) == 1 && strings.EqualFold(strings.TrimSpace(matches[0].Name), query) {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/contacts/"+strconv.FormatInt(matches[0].ID, 10), http.StatusSeeOther)
		return
	}
	if len(matches) > 0 {
		s.handleApp(w, r, tenant, appViewState{
			Flash:         "Search results for \"" + query + "\".",
			UniversalText: query,
			SearchResults: matches,
		})
		return
	}

	if looksLikeContactName(query) {
		if err := db.CreateContact(query, "", "", "", ""); err != nil {
			s.handleApp(w, r, tenant, appViewState{Flash: "Could not create contact: " + err.Error(), UniversalText: query})
			return
		}
		createdMatches, _ := db.SearchContacts(query, 1)
		if len(createdMatches) == 1 {
			http.Redirect(w, r, "/t/"+tenant.Slug+"/contacts/"+strconv.FormatInt(createdMatches[0].ID, 10), http.StatusSeeOther)
			return
		}
		s.handleApp(w, r, tenant, appViewState{Flash: "Contact created from universal input.", UniversalText: query})
		return
	}

	s.handleApp(w, r, tenant, appViewState{
		Flash:         "No contact matched. If this is a follow-up note, choose a contact in Log Interaction.",
		UniversalText: query,
	})
}

func (s *Server) handleContactDetail(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	contactID, ok := parseContactIDFromRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.handleContactDetailWithFlash(w, r, tenant, contactID, "")
}

func (s *Server) handleUpdateContact(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !s.requireCSRF(w, r) {
		return
	}
	contactID, ok := parseContactIDFromUpdateRest(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var payload struct {
		Field string `json:"field"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	updatedAt, err := db.UpdateContactField(contactID, payload.Field, payload.Value)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"updated_at": updatedAt,
	})
}

func (s *Server) handleInvitePage(w http.ResponseWriter, r *http.Request, tenant control.Tenant, rest string) {
	token, ok := parseInviteToken(rest)
	if !ok {
		http.NotFound(w, r)
		return
	}
	db, err := tenantdb.Open(tenant.DBPath)
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

	db, err := tenantdb.Open(tenant.DBPath)
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

func parseMaybeMultipartForm(r *http.Request) error {
	// Our auth flows submit using fetch(FormData), which defaults to multipart/form-data.
	// Calling ParseForm first will *not* parse multipart bodies but will initialize r.Form,
	// which prevents FormValue from later calling ParseMultipartForm.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if errors.Is(err, http.ErrNotMultipart) {
			return r.ParseForm()
		}
		return err
	}
	return nil
}

func (s *Server) handleContactDetailWithFlash(w http.ResponseWriter, r *http.Request, tenant control.Tenant, contactID int64, flash string) {
	sess, ok := s.readSession(r)
	if !ok || sess.TenantSlug != tenant.Slug {
		http.Redirect(w, r, "/t/"+tenant.Slug+"/login", http.StatusSeeOther)
		return
	}

	db, err := tenantdb.Open(tenant.DBPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	contact, err := db.ContactByID(contactID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	timeline, err := db.ListInteractionsByContact(contactID, 200)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	header := renderContactHeader(tenant, contact)
	body := renderContactDetailBody(tenant, contact, timeline, flash)
	csrf := s.ensureCSRFCookie(w, r)
	_ = s.tenantApp.ExecuteTemplate(w, "page", pageData{
		Title:     contact.Name,
		Header:    header,
		MainID:    "main-content",
		MainClass: "max-w-4xl mx-auto px-4 py-6 lg:px-6",
		Body:      body,
		CSRFToken: csrf,
	})
}

type session struct {
	TenantSlug string
	UserID     int64
}

func (s *Server) writeSession(w http.ResponseWriter, r *http.Request, sess session) error {
	payload := sess.TenantSlug + "|" + strconv.FormatInt(sess.UserID, 10)
	sig := sign(payload, s.sessionKey)
	raw := payload + "|" + sig
	cookie := &http.Cookie{
		Name:     "attention_session",
		Value:    base64.RawURLEncoding.EncodeToString([]byte(raw)),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
	}
	http.SetCookie(w, cookie)
	s.ensureCSRFCookie(w, r)
	return nil
}

func (s *Server) storeFlow(flow ceremonyFlow) string {
	token := make([]byte, 24)
	_, _ = rand.Read(token)
	id := base64.RawURLEncoding.EncodeToString(token)

	s.flowMu.Lock()
	defer s.flowMu.Unlock()
	now := time.Now().UTC()
	for key, existing := range s.webauthnFlow {
		if now.After(existing.ExpiresAt) {
			delete(s.webauthnFlow, key)
		}
	}
	s.webauthnFlow[id] = flow
	return id
}

func (s *Server) consumeFlow(flowID string) (ceremonyFlow, bool) {
	s.flowMu.Lock()
	defer s.flowMu.Unlock()
	flow, ok := s.webauthnFlow[flowID]
	if !ok {
		return ceremonyFlow{}, false
	}
	delete(s.webauthnFlow, flowID)
	if time.Now().UTC().After(flow.ExpiresAt) {
		return ceremonyFlow{}, false
	}
	return flow, true
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) readSession(r *http.Request) (session, bool) {
	cookie, err := r.Cookie("attention_session")
	if err != nil {
		return session{}, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		return session{}, false
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 {
		return session{}, false
	}
	payload := parts[0] + "|" + parts[1]
	if !hmac.Equal([]byte(sign(payload, s.sessionKey)), []byte(parts[2])) {
		return session{}, false
	}
	userID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return session{}, false
	}
	return session{TenantSlug: parts[0], UserID: userID}, true
}

func parseTenantPath(path string) (slug, rest string, ok bool) {
	if !strings.HasPrefix(path, "/t/") {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(path, "/t/"), "/", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", "", false
	}
	slug = parts[0]
	rest = "/" + parts[1]
	return slug, rest, true
}

func parseContactIDFromRest(rest string) (int64, bool) {
	trimmed := strings.TrimPrefix(rest, "/contacts/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return 0, false
	}
	if strings.Contains(trimmed, "/") {
		parts := strings.Split(trimmed, "/")
		if len(parts) != 2 || parts[1] != "interactions" {
			return 0, false
		}
		trimmed = parts[0]
	}
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func parseContactIDFromUpdateRest(rest string) (int64, bool) {
	trimmed := strings.TrimPrefix(rest, "/contacts/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return 0, false
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || parts[1] != "update" {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func parseInviteToken(rest string) (string, bool) {
	trimmed := strings.TrimPrefix(rest, "/invite/")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", false
	}
	if strings.Contains(trimmed, "/") {
		parts := strings.Split(trimmed, "/")
		if len(parts) != 3 || parts[1] != "passkey" || (parts[2] != "start" && parts[2] != "finish") {
			return "", false
		}
		trimmed = parts[0]
	}
	if len(trimmed) < 10 {
		return "", false
	}
	return trimmed, true
}

func loadOrCreateSessionKey(dataDir string) ([]byte, error) {
	path := filepath.Join(dataDir, "session.key")
	if b, err := os.ReadFile(path); err == nil && len(b) > 0 {
		return b, nil
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func sign(payload string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

type pageData struct {
	Title     string
	Header    template.HTML
	MainID    string
	MainClass string
	Body      template.HTML
	CSRFToken string
}

type appViewState struct {
	Flash         string
	SearchResults []tenantdb.Contact
	UniversalText string
	InviteLink    string
	SuggestNote   *noteSuggestion
	Duplicates    []tenantdb.DuplicateCandidate
}

type noteSuggestion struct {
	Content      string
	DueAtLocal   string
	ContactHints []tenantdb.Contact
}

const rootPageTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Attention CRM</title>
  <link rel="stylesheet" href="/static/tailwind.css">
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap" rel="stylesheet">
</head>
<body class="bg-gray-50 font-sans">
  <div class="min-h-screen">
    <div class="max-w-2xl mx-auto px-6 py-10">
      <div class="flex items-center space-x-2">
        <div class="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center">
          <span class="text-white text-sm font-semibold">A</span>
        </div>
        <span class="text-xl font-semibold text-gray-900">Attention CRM</span>
      </div>
      <div class="mt-8 bg-white rounded-2xl shadow-sm border border-gray-200 p-6">
        {{ if eq .TenantCount 0 }}
          <p class="text-sm text-gray-700">No workspace is configured yet.</p>
          <p class="mt-4"><a class="text-sm font-medium text-blue-600 hover:text-blue-700 hover:underline" href="/setup">Run initial setup</a></p>
        {{ else }}
          <p class="text-sm text-gray-700">Tenants exist. Open a tenant login URL directly, for example: <code class="px-2 py-1 rounded bg-gray-50 border border-gray-200">/t/my-org/login</code>.</p>
        {{ end }}
      </div>
    </div>
  </div>
</body>
</html>`

const tenantBaseTemplate = `{{define "page"}}<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  {{if .CSRFToken}}<meta name="attention-csrf" content="{{.CSRFToken}}">{{end}}
  <link rel="stylesheet" href="/static/tailwind.css">
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap" rel="stylesheet">
  <script>
    window.attentionCsrfToken = function () {
      var m = document.querySelector('meta[name="attention-csrf"]');
      return m ? (m.getAttribute("content") || "") : "";
    };
  </script>
</head>
<body class="bg-gray-50 font-sans">
  {{template "body" .}}
</body>
</html>{{end}}`

const tenantAuthTemplate = `{{define "body"}}
  <div class="min-h-screen flex items-center justify-center px-6 py-10">
    <div class="w-full max-w-md">
      <div class="flex items-center justify-center space-x-2">
        <div class="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center">
          <span class="text-white text-sm font-semibold">A</span>
        </div>
        <span class="text-xl font-semibold text-gray-900">Attention CRM</span>
      </div>
      <div class="mt-8 bg-white rounded-2xl shadow-sm border border-gray-200 p-6">
        {{.Body}}
      </div>
    </div>
  </div>
{{end}}`

const tenantAppTemplate = `{{define "body"}}
  <div id="app-container" class="min-h-screen bg-gray-50">
    {{if .Header}}
      {{.Header}}
    {{else}}
      <header id="header" class="bg-white border-b border-gray-200 px-6 py-4">
        <div class="flex items-center justify-between max-w-7xl mx-auto">
          <div class="flex items-center space-x-8">
            <a href="#" class="flex items-center space-x-2">
              <div class="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center">
                <svg class="w-4 h-4 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                  <path d="M13 2 3 14h7l-1 8 12-14h-7l1-6z"></path>
                </svg>
              </div>
              <span class="text-xl font-semibold text-gray-900">Attention CRM</span>
            </a>
          </div>
          <div class="flex items-center space-x-4">
            <button type="button" class="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg" aria-label="Notifications">
              <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <path d="M12 22a2 2 0 0 0 2-2h-4a2 2 0 0 0 2 2zm6-6V11a6 6 0 0 0-5-5.91V4a1 1 0 1 0-2 0v1.09A6 6 0 0 0 6 11v5l-2 2v1h16v-1l-2-2z"></path>
              </svg>
            </button>
            <div class="w-8 h-8 rounded-full bg-gray-200"></div>
          </div>
        </div>
      </header>
    {{end}}
    <main id="{{if .MainID}}{{.MainID}}{{else}}main-workspace{{end}}" class="{{if .MainClass}}{{.MainClass}}{{else}}max-w-7xl mx-auto px-6 py-8{{end}}">
      {{.Body}}
    </main>
  </div>
{{end}}`

func renderTenantAppBody(
	tenant control.Tenant,
	userID int64,
	state appViewState,
	contacts []tenantdb.Contact,
	needsAttention []tenantdb.Interaction,
	recent []tenantdb.Interaction,
) template.HTML {
	var b strings.Builder
	now := time.Now()
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)

	if state.Flash != "" {
		b.WriteString(`<div class="mb-6 bg-blue-50 border border-blue-200 rounded-lg p-3 text-sm text-blue-900">` + template.HTMLEscapeString(state.Flash) + `</div>`)
	}

	// Universal action surface.
	b.WriteString(`<div id="universal-action-surface" class="mb-8"><div class="relative"><div id="omni-card" class="relative bg-white rounded-2xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<form id="omni-form" method="POST" action="/t/` + tenantSlugEsc + `/universal"><div class="flex items-center space-x-4">`)
	b.WriteString(`<svg class="w-5 h-5 text-gray-400" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" id="omni-icon"><path fill="currentColor" d="M416 208c0 45.9-14.9 88.3-40 122.7L502.6 457.4c12.5 12.5 12.5 32.8 0 45.3s-32.8 12.5-45.3 0L330.7 376c-34.4 25.2-76.8 40-122.7 40C93.1 416 0 322.9 0 208S93.1 0 208 0S416 93.1 416 208zM208 352a144 144 0 1 0 0-288 144 144 0 1 0 0 288z"></path></svg>`)
	b.WriteString(`<input id="omni-input" name="q" type="text" value="` + template.HTMLEscapeString(state.UniversalText) + `" placeholder="Search contacts, deals, or add a quick note..." class="flex-1 text-lg bg-transparent border-none outline-none placeholder-gray-400 text-gray-900" autocomplete="off" spellcheck="false">`)
	b.WriteString(`</div></form>`)

	b.WriteString(`<div id="search-suggestions" class="mt-4 space-y-1 border-t border-gray-100 pt-4 hidden"></div>`)
	b.WriteString(`</div></div></div>`)

	// Client-side omnibar palette.
	b.WriteString(`<script>
(function(){
  var tenantSlug = "` + tenantSlugEsc + `";
  var input = document.getElementById("omni-input");
  var form = document.getElementById("omni-form");
  var card = document.getElementById("omni-card");
  var icon = document.getElementById("omni-icon");
  var panel = document.getElementById("search-suggestions");
  if(!input || !form || !panel || !card || !icon) return;

  var items = [];
  var selected = 0;
  var open = false;
  var timer = null;
  var lastQuery = "";

  function setOpen(isOpen){
    open = isOpen;
    panel.classList.toggle("hidden", !isOpen);
    card.classList.toggle("shadow-lg", isOpen);
    card.classList.toggle("border-2", isOpen);
    card.classList.toggle("border-blue-500", isOpen);
    card.classList.toggle("ring-2", isOpen);
    card.classList.toggle("ring-blue-500", isOpen);
    card.classList.toggle("ring-opacity-20", isOpen);
    icon.classList.toggle("text-gray-400", !isOpen);
    icon.classList.toggle("text-blue-600", isOpen);
  }

  function escHtml(s){
    return (s||"").replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/\"/g,"&quot;").replace(/'/g,"&#39;");
  }

  function render(){
    if(!open){ panel.innerHTML = ""; return; }
    var html = '<div class="text-xs font-medium text-gray-500 uppercase tracking-wider mb-3">Search Results</div>';
    items.forEach(function(it, idx){
      if(it.kind === "contact"){
        var rowClass = 'flex items-center space-x-3 p-3 hover:bg-gray-50 rounded-lg cursor-pointer transition-colors';
        if(idx === selected){
          rowClass = 'flex items-center space-x-3 p-3 bg-blue-50 border border-blue-200 rounded-lg cursor-pointer hover:bg-blue-100 transition-colors';
        }
        html += '<div class="'+rowClass+'" data-idx="'+idx+'">' +
          '<div class="w-8 h-8 bg-blue-600 rounded-full flex items-center justify-center"><span class="text-white text-xs font-semibold">'+escHtml(it.initials)+'</span></div>' +
          '<div class="flex-1">' +
            '<div class="flex items-center space-x-2">' +
              '<span class="text-sm font-medium text-gray-900">'+escHtml(it.name)+'</span>' +
              (it.company ? '<span class="text-xs text-gray-500">•</span><span class="text-xs text-gray-600">'+escHtml(it.company)+'</span>' : '') +
            '</div>' +
            (it.subline ? '<div class="text-xs text-gray-500">'+escHtml(it.subline)+'</div>' : '') +
          '</div>' +
          '<div class="text-xs text-blue-600 font-medium">Open</div>' +
        '</div>';
        return;
      }
      if(it.kind === "create_contact"){
        var rowClass2 = 'flex items-center space-x-3 p-3 hover:bg-gray-50 rounded-lg cursor-pointer transition-colors';
        if(idx === selected){
          rowClass2 = 'flex items-center space-x-3 p-3 bg-blue-50 border border-blue-200 rounded-lg cursor-pointer hover:bg-blue-100 transition-colors';
        }
        html += '<div class="'+rowClass2+'" data-idx="'+idx+'">' +
          '<div class="w-8 h-8 bg-green-600 rounded-full flex items-center justify-center">' +
            '<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M19 11H13V5h-2v6H5v2h6v6h2v-6h6v-2Z"/></svg>' +
          '</div>' +
          '<div class="flex-1">' +
            '<div class="text-sm font-medium text-gray-900">Create contact: '+escHtml(it.name)+'</div>' +
            '<div class="text-xs text-gray-500">Add a new contact record</div>' +
          '</div>' +
          '<div class="text-xs text-green-600 font-medium">Create</div>' +
        '</div>';
        return;
      }
    });
    panel.innerHTML = html;
  }

  function setItems(result){
    items = [];
    (result.contacts || []).forEach(function(c){
      items.push({
        kind: "contact",
        id: c.id,
        name: c.name,
        company: c.company || "",
        initials: (c.name || "?").split(/\s+/).filter(Boolean).slice(0,2).map(function(p){return p[0]||"";}).join("").toUpperCase() || "?",
        subline: ""
      });
    });
    (result.actions || []).forEach(function(a){
      if(a.type === "create_contact"){
        items.push({kind:"create_contact", name: a.name});
      }
    });
    selected = 0;
    setOpen(items.length > 0);
    render();
  }

  function activate(idx){
    var it = items[idx];
    if(!it) return;
    if(it.kind === "contact"){
      window.location.href = "/t/" + tenantSlug + "/contacts/" + it.id;
      return;
    }
    if(it.kind === "create_contact"){
      // Submit to the explicit quick-create endpoint (no auto-create).
      var f = document.createElement("form");
      f.method = "POST";
      f.action = "/t/" + tenantSlug + "/contacts/quick";
      var inp = document.createElement("input");
      inp.type = "hidden";
      inp.name = "name";
      inp.value = it.name;
      f.appendChild(inp);
      document.body.appendChild(f);
      f.submit();
      return;
    }
  }

  function fetchResults(){
    var q = (input.value || "").trim();
    lastQuery = q;
    if(q === ""){
      setOpen(false);
      panel.innerHTML = "";
      items = [];
      return;
    }
    fetch("/t/" + tenantSlug + "/omni?q=" + encodeURIComponent(q), {headers: {"Accept":"application/json"}})
      .then(function(r){ if(!r.ok) throw new Error("bad"); return r.json(); })
      .then(function(data){ if((input.value||"").trim() !== lastQuery) return; setItems(data); })
      .catch(function(){ setOpen(false); });
  }

  input.addEventListener("keydown", function(e){
    if(!open){
      if(e.key === "Enter"){
        e.preventDefault();
        fetchResults();
      }
      return;
    }
    if(e.key === "ArrowDown"){
      e.preventDefault();
      selected = Math.min(items.length-1, selected+1);
      render();
      return;
    }
    if(e.key === "ArrowUp"){
      e.preventDefault();
      selected = Math.max(0, selected-1);
      render();
      return;
    }
    if(e.key === "Enter"){
      e.preventDefault();
      activate(selected);
      return;
    }
    if(e.key === "Escape"){
      e.preventDefault();
      setOpen(false);
      return;
    }
  });

  input.addEventListener("input", function(){
    if(timer) clearTimeout(timer);
    timer = setTimeout(fetchResults, 120);
  });

  document.addEventListener("click", function(e){
    if(!card.contains(e.target)){
      setOpen(false);
      return;
    }
    var row = e.target && e.target.closest ? e.target.closest("[data-idx]") : null;
    if(!row) return;
    var idx = parseInt(row.getAttribute("data-idx") || "0", 10);
    selected = idx;
    render();
    activate(idx);
  });
})();
</script>`)

	// Possible duplicates (kept, but styled in the new system).
	if len(state.Duplicates) > 0 {
		b.WriteString(`<div class="bg-white rounded-xl shadow-sm border border-gray-200 p-6 mb-8">`)
		b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-3">Possible duplicates</div>`)
		b.WriteString(`<div class="space-y-2">`)
		for _, d := range state.Duplicates {
			label := d.Contact.Name
			if d.Contact.Company != "" {
				label += " • " + d.Contact.Company
			}
			b.WriteString(`<div class="flex items-center justify-between p-3 hover:bg-gray-50 rounded-lg">`)
			b.WriteString(`<a class="text-sm font-medium text-gray-900 hover:underline" href="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(d.Contact.ID, 10) + `">` + template.HTMLEscapeString(label) + `</a>`)
			b.WriteString(`<div class="text-xs text-gray-500">` + template.HTMLEscapeString(d.Reason) + `</div>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div></div>`)
	}

	// Quick capture section.
	b.WriteString(`<div id="quick-capture-section" class="mt-6"><div class="grid grid-cols-4 gap-4">`)
	b.WriteString(quickCaptureButton("New Contact", "Add person or company", "hover:border-blue-300 hover:bg-blue-50", "bg-blue-100", "group-hover:bg-blue-200", "text-blue-600", "M12 5a3 3 0 1 0 0 6 3 3 0 0 0 0-6Zm-7 14c0-3.314 2.686-6 6-6h2c3.314 0 6 2.686 6 6v1H5v-1Zm13-6v-2h2V9h-2V7h-2v2h-2v2h2v2h2Z"))
	b.WriteString(quickCaptureButton("Log Call", "Record conversation", "hover:border-green-300 hover:bg-green-50", "bg-green-100", "group-hover:bg-green-200", "text-green-600", "M6.62 10.79a15.053 15.053 0 0 0 6.59 6.59l2.2-2.2a1 1 0 0 1 1.01-.24c1.12.37 2.33.57 3.58.57a1 1 0 0 1 1 1V20a1 1 0 0 1-1 1C10.61 21 3 13.39 3 4a1 1 0 0 1 1-1h3.5a1 1 0 0 1 1 1c0 1.25.2 2.46.57 3.59a1 1 0 0 1-.24 1.01l-2.21 2.19Z"))
	b.WriteString(quickCaptureButton("Quick Note", "Capture thoughts", "hover:border-yellow-300 hover:bg-yellow-50", "bg-yellow-100", "group-hover:bg-yellow-200", "text-yellow-600", "M6 2h9l5 5v15a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Zm8 1.5V8h4.5L14 3.5Z"))
	b.WriteString(quickCaptureButton("New Deal", "Track opportunity", "hover:border-purple-300 hover:bg-purple-50", "bg-purple-100", "group-hover:bg-purple-200", "text-purple-600", "M20 6h-3.586l-1.707-1.707A1 1 0 0 0 14 4H10a1 1 0 0 0-.707.293L7.586 6H4a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2Zm0 12H4V8h4l2-2h4l2 2h4v10Z"))
	b.WriteString(`</div></div>`)

	b.WriteString(`<div class="grid grid-cols-12 gap-6 mt-6" id="content-grid">`)

	// Needs Attention
	b.WriteString(`<div id="needs-attention-section" class="col-span-5"><div class="bg-white rounded-xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<div class="flex items-center justify-between mb-6"><h2 class="text-lg font-semibold text-gray-900">Needs Attention</h2>`)
	b.WriteString(`<span class="bg-red-100 text-red-800 text-xs font-medium px-2 py-1 rounded-full">` + strconv.Itoa(len(needsAttention)) + `</span></div>`)
	if len(needsAttention) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No follow-ups due.</div>`)
	} else {
		b.WriteString(`<div class="space-y-4">`)
		for _, it := range needsAttention {
			bgClass, iconClass, meta, actionText := attentionItemMeta(it, now)
			b.WriteString(`<div class="flex items-start space-x-3 p-3 ` + bgClass + ` rounded-lg">`)
			b.WriteString(`<svg class="w-4 h-4 ` + iconClass + ` mt-1" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 8v5l4 2-1 1.732L10 14V8h2Zm0-6a10 10 0 1 0 0 20 10 10 0 0 0 0-20Z"/></svg>`)
			b.WriteString(`<div class="flex-1">`)
			b.WriteString(`<p class="text-sm font-medium text-gray-900">Follow up with ` + template.HTMLEscapeString(it.ContactName) + `</p>`)
			b.WriteString(`<p class="text-xs mt-1 ` + iconClass + `">` + template.HTMLEscapeString(meta) + `</p>`)
			b.WriteString(`</div>`)
			b.WriteString(`<a class="text-xs font-medium hover:underline ` + iconClass + `" href="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(it.ContactID, 10) + `">` + template.HTMLEscapeString(actionText) + `</a>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></div>`)

	// Recent Interactions
	b.WriteString(`<div id="recent-interactions-section" class="col-span-7"><div class="bg-white rounded-xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<h2 class="text-lg font-semibold text-gray-900 mb-6">Recent Interactions</h2>`)
	if len(recent) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No interactions yet.</div>`)
	} else {
		b.WriteString(`<div class="space-y-4">`)
		for _, it := range recent {
			title, desc := splitTitleDesc(it.Content)
			b.WriteString(`<a href="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(it.ContactID, 10) + `" class="flex items-center space-x-4 p-3 hover:bg-gray-50 rounded-lg cursor-pointer">`)
			b.WriteString(`<div class="w-10 h-10 bg-blue-600 rounded-full flex items-center justify-center"><span class="text-white text-xs font-semibold">` + template.HTMLEscapeString(initials(it.ContactName)) + `</span></div>`)
			b.WriteString(`<div class="flex-1">`)
			b.WriteString(`<p class="text-sm font-medium text-gray-900">` + template.HTMLEscapeString(it.ContactName) + `</p>`)
			line := title
			if desc != "" {
				line = title + " — " + desc
			}
			b.WriteString(`<p class="text-xs text-gray-600 mt-1">` + template.HTMLEscapeString(snippet(line, 120)) + `</p>`)
			b.WriteString(`</div>`)
			b.WriteString(`<div class="text-xs text-gray-500">` + template.HTMLEscapeString(relativeTime(it.CreatedAt, now)) + `</div>`)
			b.WriteString(`</a>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></div>`)

	b.WriteString(`</div>`)

	return template.HTML(b.String())
}

func inviteStatus(inv tenantdb.Invite) string {
	if inv.RevokedAt.Valid {
		return "Revoked"
	}
	if inv.RedeemedAt.Valid {
		return "Redeemed"
	}
	if inv.StartedAt.Valid {
		return "Started"
	}
	return "Pending"
}

func renderMembersBody(tenant control.Tenant, userID int64, members []tenantdb.Member, invites []tenantdb.Invite) template.HTML {
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)

	var b strings.Builder
	b.WriteString(`<div class="max-w-4xl mx-auto">`)
	b.WriteString(`<div class="flex items-center justify-between mb-6">`)
	b.WriteString(`<div>`)
	b.WriteString(`<h1 class="text-2xl font-semibold text-gray-900">Members</h1>`)
	b.WriteString(`<p class="mt-1 text-sm text-gray-600">Manage workspace members and pending invites.</p>`)
	b.WriteString(`</div>`)
	b.WriteString(`<a class="text-sm font-medium text-blue-600 hover:text-blue-700 hover:underline" href="/t/` + tenantSlugEsc + `/app">Back to app</a>`)
	b.WriteString(`</div>`)

	// Members
	b.WriteString(`<div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6 mb-6">`)
	b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-4">Current members</div>`)
	if len(members) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No members found.</div>`)
	} else {
		b.WriteString(`<div class="divide-y divide-gray-100">`)
		for _, m := range members {
			role := "Member"
			if m.IsOwner {
				role = "Owner"
			}
			initial := "?"
			if s := strings.TrimSpace(m.Name); s != "" {
				initial = strings.ToUpper(string([]rune(s)[0]))
			}
			b.WriteString(`<div class="py-3 flex items-center gap-3">`)
			b.WriteString(`<div class="w-9 h-9 rounded-full bg-gray-100 flex items-center justify-center text-sm font-semibold text-gray-600">` + template.HTMLEscapeString(initial) + `</div>`)
			b.WriteString(`<div class="flex-1 min-w-0">`)
			b.WriteString(`<div class="flex items-center gap-2">`)
			b.WriteString(`<div class="text-sm font-medium text-gray-900 truncate">` + template.HTMLEscapeString(m.Name) + `</div>`)
			if m.IsOwner {
				b.WriteString(`<span class="text-xs font-medium rounded-full bg-blue-50 text-blue-700 px-2 py-0.5">Owner</span>`)
			}
			b.WriteString(`</div>`)
			b.WriteString(`<div class="text-xs text-gray-500 truncate">` + template.HTMLEscapeString(m.Email) + `</div>`)
			b.WriteString(`</div>`)
			b.WriteString(`<div class="text-xs text-gray-500">` + template.HTMLEscapeString(role) + `</div>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	// Create invite (reuse existing behavior; link only shown after create on /app today).
	b.WriteString(`<div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6 mb-6">`)
	b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-1">Invite a teammate</div>`)
	b.WriteString(`<div class="text-sm text-gray-600 mb-4">Creates an invite link (no SMTP). The link is shown once after creation.</div>`)
	b.WriteString(`<form method="POST" action="/t/` + tenantSlugEsc + `/invites" class="flex items-end gap-3">`)
	b.WriteString(`<div class="flex-1">`)
	b.WriteString(`<label class="block text-sm font-medium text-gray-700">Email</label>`)
	b.WriteString(`<input name="email" type="email" required class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" placeholder="teammate@company.com">`)
	b.WriteString(`</div>`)
	b.WriteString(`<button type="submit" class="h-10 px-5 rounded-xl bg-blue-600 text-white font-medium hover:bg-blue-700">Create invite</button>`)
	b.WriteString(`</form>`)
	b.WriteString(`</div>`)

	// Invites list
	b.WriteString(`<div class="bg-white rounded-2xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-4">Invites</div>`)
	if len(invites) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No invites yet.</div>`)
	} else {
		b.WriteString(`<div class="space-y-2">`)
		for _, inv := range invites {
			status := inviteStatus(inv)
			statusClass := "bg-gray-50 text-gray-700"
			if status == "Pending" {
				statusClass = "bg-yellow-50 text-yellow-800"
			} else if status == "Started" {
				statusClass = "bg-blue-50 text-blue-700"
			} else if status == "Redeemed" {
				statusClass = "bg-green-50 text-green-700"
			} else if status == "Revoked" {
				statusClass = "bg-red-50 text-red-700"
			}

			b.WriteString(`<div class="flex items-center justify-between gap-4 p-3 rounded-xl border border-gray-200">`)
			b.WriteString(`<div class="min-w-0">`)
			b.WriteString(`<div class="text-sm font-medium text-gray-900 truncate">` + template.HTMLEscapeString(inv.Email) + `</div>`)
			b.WriteString(`<div class="text-xs text-gray-500">Created: ` + template.HTMLEscapeString(inv.CreatedAt) + ` • Expires: ` + template.HTMLEscapeString(inv.ExpiresAt) + `</div>`)
			b.WriteString(`</div>`)
			b.WriteString(`<div class="flex items-center gap-3">`)
			b.WriteString(`<span class="text-xs font-medium rounded-full px-2 py-0.5 ` + statusClass + `">` + template.HTMLEscapeString(status) + `</span>`)
			if status == "Pending" || status == "Started" {
				b.WriteString(`<form method="POST" action="/t/` + tenantSlugEsc + `/invites/` + strconv.FormatInt(inv.ID, 10) + `/revoke" style="margin:0">`)
				b.WriteString(`<button type="submit" class="text-sm font-medium text-red-600 hover:text-red-700 hover:underline">Revoke</button>`)
				b.WriteString(`</form>`)
			}
			b.WriteString(`</div>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	b.WriteString(`</div>`)

	return template.HTML(b.String())
}

func quickCaptureButton(title, subtitle, hoverClass, iconBgClass, iconBgHoverClass, iconClass, iconPath string) string {
	return `<button type="button" class="bg-white border border-gray-200 rounded-xl p-6 ` + hoverClass + ` transition-all duration-200 group">
  <div class="flex flex-col items-center text-center space-y-3">
    <div class="w-12 h-12 ` + iconBgClass + ` rounded-full flex items-center justify-center ` + iconBgHoverClass + `">
      <svg class="w-5 h-5 ` + iconClass + `" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="` + iconPath + `"></path></svg>
    </div>
    <div>
      <p class="text-sm font-medium text-gray-900">` + template.HTMLEscapeString(title) + `</p>
      <p class="text-xs text-gray-500 mt-1">` + template.HTMLEscapeString(subtitle) + `</p>
    </div>
  </div>
</button>`
}

func initials(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return "?"
	}
	if len(parts) == 1 {
		r := []rune(parts[0])
		if len(r) == 0 {
			return "?"
		}
		if len(r) == 1 {
			return strings.ToUpper(string(r[0]))
		}
		return strings.ToUpper(string(r[0:2]))
	}
	a := []rune(parts[0])
	b := []rune(parts[len(parts)-1])
	if len(a) == 0 || len(b) == 0 {
		return "?"
	}
	return strings.ToUpper(string([]rune{a[0], b[0]}))
}

func snippet(text string, max int) string {
	t := strings.TrimSpace(text)
	r := []rune(t)
	if len(r) <= max {
		return t
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}

func parseRFC3339(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func relativeTime(createdAt string, now time.Time) string {
	t, ok := parseRFC3339(createdAt)
	if !ok {
		return createdAt
	}
	d := now.Sub(t)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " minutes ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + " hours ago"
	case d < 48*time.Hour:
		return "Yesterday"
	case d < 7*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + " days ago"
	default:
		return t.Format("Jan 2, 2006")
	}
}

func attentionItemMeta(it tenantdb.Interaction, now time.Time) (bgClass, iconClass, meta, actionText string) {
	bgClass = "bg-amber-50 border border-amber-200"
	iconClass = "text-amber-600"
	meta = "Due soon"
	actionText = "Act"
	if it.DueAt.Valid {
		if dueT, ok := parseRFC3339(it.DueAt.String); ok {
			if dueT.Before(now) {
				bgClass = "bg-red-50 border border-red-200"
				iconClass = "text-red-600"
				actionText = "Act"
				over := now.Sub(dueT)
				if over < time.Hour {
					meta = "Overdue by " + strconv.Itoa(int(over.Minutes())) + " minutes"
				} else if over < 24*time.Hour {
					meta = "Overdue by " + strconv.Itoa(int(over.Hours())) + " hours"
				} else {
					meta = "Overdue by " + strconv.Itoa(int(over.Hours()/24)) + " days"
				}
				return
			}
			until := dueT.Sub(now)
			if until < time.Hour {
				meta = "Due in " + strconv.Itoa(int(until.Minutes())) + " minutes"
				actionText = "Start"
			} else if until < 24*time.Hour {
				meta = "Due in " + strconv.Itoa(int(until.Hours())) + " hours"
				actionText = "Start"
			} else {
				meta = "Due in " + strconv.Itoa(int(until.Hours()/24)) + " days"
				actionText = "Review"
			}
			return
		}
		meta = "Due: " + it.DueAt.String
		actionText = "Act"
	}
	return
}

func splitTitleDesc(content string) (string, string) {
	t := strings.TrimSpace(content)
	if t == "" {
		return "", ""
	}
	// Prefer splitting on first sentence boundary.
	for _, sep := range []string{". ", "\n"} {
		if idx := strings.Index(t, sep); idx > 0 {
			title := strings.TrimSpace(t[:idx+1])
			desc := strings.TrimSpace(t[idx+1:])
			if desc != "" {
				desc = strings.TrimLeft(desc, ". \n\t")
			}
			return title, desc
		}
	}
	return t, ""
}

func renderContactDetailBody(
	tenant control.Tenant,
	contact tenantdb.Contact,
	timeline []tenantdb.Interaction,
	flash string,
) template.HTML {
	var b strings.Builder
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)
	now := time.Now()

	if flash != "" {
		b.WriteString(`<div class="mb-6 bg-blue-50 border border-blue-200 rounded-lg p-3 text-sm text-blue-900">` + template.HTMLEscapeString(flash) + `</div>`)
	}

	// Identity card.
	b.WriteString(`<div id="identity-card" class="bg-white rounded-xl shadow-sm border border-gray-200 p-6 mb-6">`)
	b.WriteString(`<div class="space-y-4">`)
	b.WriteString(identityRow("email", "envelope", "email", contact.Email, "Add email"))
	b.WriteString(identityRow("phone", "phone", "tel", contact.Phone, "Add phone"))
	b.WriteString(identityRow("company", "building", "text", contact.Company, "Add company"))
	b.WriteString(`<button type="button" class="text-sm text-blue-600 hover:text-blue-700 font-medium hover:underline flex items-center space-x-2" id="add-more-btn"><span>+</span><span>Add more</span></button>`)
	b.WriteString(`<div id="optional-fields" class="hidden pt-4 border-t border-gray-100">`)
	b.WriteString(`<div class="space-y-3">`)
	b.WriteString(`<button type="button" class="w-full text-left text-sm text-blue-600 hover:text-blue-700 font-medium hover:underline flex items-center space-x-2"><span class="text-blue-600">+</span><span>Add job title</span></button>`)
	b.WriteString(`<button type="button" class="w-full text-left text-sm text-blue-600 hover:text-blue-700 font-medium hover:underline flex items-center space-x-2"><span class="text-blue-600">+</span><span>Add location</span></button>`)
	b.WriteString(`<button type="button" class="w-full text-left text-sm text-blue-600 hover:text-blue-700 font-medium hover:underline flex items-center space-x-2"><span class="text-blue-600">+</span><span>Add LinkedIn profile</span></button>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div></div>`)

	// Interaction composer.
	b.WriteString(`<div id="interaction-composer" class="bg-white rounded-xl shadow-sm border border-gray-200 p-6 mb-6">`)
	b.WriteString(`<form method="POST" action="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(contact.ID, 10) + `/interactions">`)
	b.WriteString(`<div class="space-y-4">`)
	b.WriteString(`<div class="flex flex-col sm:flex-row sm:items-center gap-3">`)
	b.WriteString(`<select name="type" required class="bg-gray-50 border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500 w-full sm:w-auto">`)
	b.WriteString(`<option value="note" selected>Note</option><option value="call">Call</option><option value="email">Email</option><option value="meeting">Meeting</option>`)
	b.WriteString(`</select></div>`)
	b.WriteString(`<textarea name="content" required placeholder="What happened? Discussed Q1 budget planning. Sarah mentioned they're looking to expand..." class="w-full h-20 sm:h-24 p-3 border border-gray-200 rounded-lg resize-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 text-sm"></textarea>`)
	b.WriteString(`<div class="space-y-3"><div class="border border-gray-200 rounded-lg overflow-hidden">`)
	b.WriteString(`<label class="flex items-center space-x-2 cursor-pointer p-3 hover:bg-gray-50">`)
	b.WriteString(`<input type="checkbox" class="rounded border-gray-300 text-blue-600 focus:ring-blue-500 w-4 h-4" id="follow-up-toggle">`)
	b.WriteString(`<span class="text-sm font-medium text-gray-700">Set follow-up</span></label>`)
	b.WriteString(`<div id="follow-up-date-container" class="hidden px-3 pb-3 bg-gray-50 border-t border-gray-200">`)
	b.WriteString(`<input name="due_at" type="datetime-local" class="w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500 mt-2">`)
	b.WriteString(`</div></div></div>`)
	b.WriteString(`<div class="flex justify-end pt-2"><button class="bg-blue-600 text-white px-6 py-2.5 rounded-lg font-medium hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 text-sm w-full sm:w-auto flex items-center justify-center space-x-2"><span>Log interaction</span></button></div>`)
	b.WriteString(`</div></form></div>`)

	// Timeline.
	b.WriteString(`<div id="timeline-section" class="bg-white rounded-xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<h3 class="text-lg font-semibold text-gray-900 mb-4">Timeline</h3>`)
	if len(timeline) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No interactions yet.</div></div>`)
		return template.HTML(b.String())
	}
	b.WriteString(`<div class="space-y-4">`)
	for _, it := range timeline {
		title, desc := splitTitleDesc(it.Content)
		itemClass := `flex items-start space-x-3 p-4 hover:bg-gray-50 rounded-lg`
		chip := ``
		action := ``
		icon := interactionIcon(it.Type, "normal")
		meta := relativeTime(it.CreatedAt, now)

		if it.CompletedAt.Valid {
			itemClass = `flex items-start space-x-3 p-4 bg-green-50 border border-green-200 rounded-lg`
			chip = `<span class="bg-green-100 text-green-800 text-xs font-medium px-2 py-1 rounded-full">Completed</span>`
			icon = interactionIcon(it.Type, "completed")
		} else if it.DueAt.Valid {
			itemClass = `flex items-start space-x-3 p-4 bg-amber-50 border border-amber-200 rounded-lg`
			chip = `<span class="bg-amber-100 text-amber-800 text-xs font-medium px-2 py-1 rounded-full">Due</span>`
			icon = interactionIcon(it.Type, "due")
			meta = "Due: " + dueDisplay(it.DueAt.String, now)
			action = `<form method="POST" action="/t/` + tenantSlugEsc + `/interactions/` + strconv.FormatInt(it.ID, 10) + `/complete" style="margin:0"><button class="text-xs text-blue-600 hover:text-blue-700 font-medium hover:underline" type="submit">Mark complete</button></form>`
		}

		b.WriteString(`<div class="` + itemClass + `">`)
		if chip != "" {
			b.WriteString(`<div class="flex items-center space-x-2">` + icon + chip + `</div>`)
		} else {
			b.WriteString(icon)
		}
		b.WriteString(`<div class="flex-1">`)
		b.WriteString(`<p class="text-sm font-medium text-gray-900">` + template.HTMLEscapeString(snippet(title, 80)) + `</p>`)
		if desc != "" {
			b.WriteString(`<p class="text-xs text-gray-600 mt-1">` + template.HTMLEscapeString(snippet(desc, 200)) + `</p>`)
		}
		b.WriteString(`<p class="text-xs text-gray-500 mt-2">` + template.HTMLEscapeString(meta) + `</p>`)
		b.WriteString(`</div>`)
		if action != "" {
			b.WriteString(action)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></div>`)

	b.WriteString(`<script>
(function(){
  var tenantSlug = "` + template.HTMLEscapeString(tenant.Slug) + `";
  var contactID = ` + strconv.FormatInt(contact.ID, 10) + `;
  var updateURL = "/t/" + tenantSlug + "/contacts/" + contactID + "/update";

  var saveIndicator = document.getElementById('save-indicator');
  function setIndicator(state){
    if(!saveIndicator) return;
    if(state === "saving"){
      saveIndicator.innerHTML = '<svg class="w-4 h-4 text-gray-400 animate-spin" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 2a10 10 0 0 1 7.07 2.93l-1.41 1.41A8 8 0 1 0 20 12h2A10 10 0 0 1 12 22 10 10 0 0 1 12 2Z"/></svg><span class="text-gray-500">Saving...</span>';
      return;
    }
    if(state === "error"){
      saveIndicator.innerHTML = '<svg class="w-4 h-4 text-red-500" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20Zm1 13h-2v2h2v-2Zm0-10h-2v8h2V5Z"/></svg><span class="text-red-600">Not saved</span>';
      return;
    }
    saveIndicator.innerHTML = '<svg class="w-4 h-4 text-green-500" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4Z"/></svg><span class="text-green-600">Saved</span>';
  }

  var timers = {};
  function scheduleSave(el){
    var field = el.getAttribute('data-field');
    if(!field) return;
    var value = el.value || "";
    setIndicator("saving");
    if(timers[field]) clearTimeout(timers[field]);
    timers[field] = setTimeout(function(){ doSave(field, value); }, 450);
  }

  function doSave(field, value){
    fetch(updateURL, {
      method: "POST",
      headers: {"Content-Type":"application/json","X-CSRF-Token": (window.attentionCsrfToken ? window.attentionCsrfToken() : "")},
      body: JSON.stringify({field: field, value: value})
    }).then(function(res){
      if(!res.ok) throw new Error("bad status");
      return res.json();
    }).then(function(){
      setIndicator("saved");
    }).catch(function(){
      setIndicator("error");
    });
  }

  var fields = document.querySelectorAll('[data-field]');
  fields.forEach(function(el){
    el.addEventListener('input', function(){ scheduleSave(el); });
    el.addEventListener('blur', function(){
      var field = el.getAttribute('data-field');
      if(timers[field]) { clearTimeout(timers[field]); timers[field]=null; }
      doSave(field, el.value || "");
    });
  });

  var addMore = document.getElementById('add-more-btn');
  var optional = document.getElementById('optional-fields');
  if (addMore && optional) addMore.addEventListener('click', function(){ optional.classList.toggle('hidden'); });

  var toggle = document.getElementById('follow-up-toggle');
  var container = document.getElementById('follow-up-date-container');
  if (toggle && container) toggle.addEventListener('change', function(){ container.classList.toggle('hidden', !toggle.checked); });
})();
</script>`)

	return template.HTML(b.String())
}

func renderContactHeader(tenant control.Tenant, contact tenantdb.Contact) template.HTML {
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)
	name := template.HTMLEscapeString(contact.Name)
	return template.HTML(`
<header id="header" class="bg-white border-b border-gray-200 px-4 py-4 lg:px-6">
  <div class="flex items-center justify-between max-w-4xl mx-auto">
    <div class="flex items-center space-x-4">
      <a href="/t/` + tenantSlugEsc + `/app" class="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg" aria-label="Back">
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M14.7 6.3 13.3 4.9 6.2 12l7.1 7.1 1.4-1.4L9 12l5.7-5.7Z"/></svg>
      </a>
      <div class="flex items-center space-x-3">
        <input type="text" value="` + name + `" class="text-xl lg:text-2xl font-semibold text-gray-900 bg-transparent border-none outline-none hover:bg-gray-50 focus:bg-white focus:ring-2 focus:ring-blue-500 rounded-lg px-2 py-1" id="contact-name" data-field="name" autocomplete="off" />
        <span class="text-xs text-gray-400 font-medium flex items-center space-x-1 transition-all duration-300" id="save-indicator">
          <svg class="w-4 h-4 text-green-500" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4Z"/></svg>
          <span class="text-green-600">Saved</span>
        </span>
      </div>
    </div>
    <div class="flex items-center space-x-2">
      <button type="button" class="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg" aria-label="Menu">
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 7a2 2 0 1 0 0-4 2 2 0 0 0 0 4Zm0 7a2 2 0 1 0 0-4 2 2 0 0 0 0 4Zm0 7a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z"/></svg>
      </button>
    </div>
  </div>
</header>`)
}

func identityRow(field, icon, inputType, value, placeholder string) string {
	var iconPath string
	switch icon {
	case "envelope":
		iconPath = "M20 4H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2Zm0 4-8 5L4 8V6l8 5 8-5v2Z"
	case "phone":
		iconPath = "M6.62 10.79a15.053 15.053 0 0 0 6.59 6.59l2.2-2.2a1 1 0 0 1 1.01-.24c1.12.37 2.33.57 3.58.57a1 1 0 0 1 1 1V20a1 1 0 0 1-1 1C10.61 21 3 13.39 3 4a1 1 0 0 1 1-1h3.5a1 1 0 0 1 1 1c0 1.25.2 2.46.57 3.59a1 1 0 0 1-.24 1.01l-2.21 2.19Z"
	default: // building
		iconPath = "M4 22V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v18h-2v-2H6v2H4Zm2-4h9V4H6v14Zm2-9h2v2H8V9Zm0 4h2v2H8v-2Zm4-4h2v2h-2V9Zm0 4h2v2h-2v-2Z"
	}
	return `<div class="flex items-center space-x-3 group cursor-text">
  <svg class="w-4 h-4 text-gray-400" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="` + iconPath + `"></path></svg>
  <input type="` + template.HTMLEscapeString(inputType) + `" value="` + template.HTMLEscapeString(value) + `" class="flex-1 text-gray-900 bg-transparent border-none outline-none hover:bg-gray-50 focus:bg-white focus:ring-2 focus:ring-blue-500 rounded-lg px-2 py-1 cursor-text" placeholder="` + template.HTMLEscapeString(placeholder) + `" data-field="` + template.HTMLEscapeString(field) + `" autocomplete="off" />
  <svg class="w-3 h-3 text-gray-400 opacity-0 group-hover:opacity-100 transition-opacity" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25Zm18-11.5a1 1 0 0 0 0-1.41l-1.34-1.34a1 1 0 0 0-1.41 0l-1.13 1.13 3.75 3.75 1.13-1.13Z"/></svg>
</div>`
}

func dueDisplay(dueAt string, now time.Time) string {
	t, ok := parseRFC3339(dueAt)
	if !ok {
		return dueAt
	}
	local := t.Local()
	if local.Year() == now.Local().Year() && local.YearDay() == now.Local().YearDay() {
		return "Today at " + local.Format("3:04 PM")
	}
	if local.After(now.Add(-48*time.Hour)) && local.Before(now.Add(48*time.Hour)) {
		// approximate tomorrow/yesterday.
		if local.After(now) && local.YearDay() == now.Local().Add(24*time.Hour).YearDay() {
			return "Tomorrow at " + local.Format("3:04 PM")
		}
	}
	return local.Format("Jan 2 at 3:04 PM")
}

func interactionIcon(interactionType, variant string) string {
	path := "M6 2h9l5 5v15a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Zm8 1.5V8h4.5L14 3.5Z"
	color := "text-blue-600"
	switch interactionType {
	case "call":
		path = "M6.62 10.79a15.053 15.053 0 0 0 6.59 6.59l2.2-2.2a1 1 0 0 1 1.01-.24c1.12.37 2.33.57 3.58.57a1 1 0 0 1 1 1V20a1 1 0 0 1-1 1C10.61 21 3 13.39 3 4a1 1 0 0 1 1-1h3.5a1 1 0 0 1 1 1c0 1.25.2 2.46.57 3.59a1 1 0 0 1-.24 1.01l-2.21 2.19Z"
		color = "text-green-600"
	case "email":
		path = "M20 4H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2Zm0 4-8 5L4 8V6l8 5 8-5v2Z"
		color = "text-purple-600"
	case "meeting":
		path = "M7 2v2H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2h-2V2h-2v2H9V2H7Zm12 18H5V9h14v11ZM7 11h5v5H7v-5Z"
		color = "text-indigo-600"
	default:
		color = "text-blue-600"
	}
	if variant == "due" {
		color = "text-amber-600"
	}
	if variant == "completed" {
		color = "text-green-600"
	}
	return `<svg class="w-4 h-4 ` + color + `" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="` + path + `"></path></svg>`
}

func looksLikeContactName(input string) bool {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" || len(trimmed) > 80 {
		return false
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 || len(parts) > 4 {
		return false
	}
	stopwords := map[string]struct{}{
		"call": {}, "email": {}, "meeting": {}, "meet": {}, "tomorrow": {}, "today": {}, "next": {}, "follow": {}, "up": {},
	}
	for _, p := range parts {
		if _, found := stopwords[strings.ToLower(p)]; found {
			return false
		}
		for _, r := range p {
			if !(r == '\'' || r == '-' || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
				return false
			}
		}
	}
	return true
}

func parseDueSuggestionLocal(input string, now time.Time) (string, bool) {
	text := strings.ToLower(input)
	base := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, now.Location())

	switch {
	case strings.Contains(text, "tomorrow"):
		return base.Add(24 * time.Hour).Format("2006-01-02T15:04"), true
	case strings.Contains(text, "today"):
		return base.Format("2006-01-02T15:04"), true
	default:
		return "", false
	}
}

func looksLikeNote(input string) bool {
	t := strings.ToLower(strings.TrimSpace(input))
	if t == "" {
		return false
	}
	noteWords := []string{
		"call ",
		"email ",
		"meet ",
		"meeting ",
		"follow up",
		"follow-up",
		"remind",
		"tomorrow",
		"today",
	}
	for _, w := range noteWords {
		if strings.Contains(t, w) {
			return true
		}
	}
	return false
}

func extractContactQueryFromNote(input string) string {
	words := strings.Fields(strings.ToLower(input))
	if len(words) == 0 {
		return ""
	}
	verbs := map[string]struct{}{
		"call": {}, "email": {}, "meet": {}, "meeting": {}, "follow": {}, "up": {}, "with": {}, "remind": {}, "me": {},
	}
	stop := map[string]struct{}{"tomorrow": {}, "today": {}}

	out := make([]string, 0, len(words))
	for _, w := range words {
		if _, found := verbs[w]; found {
			continue
		}
		if _, found := stop[w]; found {
			continue
		}
		out = append(out, w)
	}
	return strings.TrimSpace(strings.Join(out, " "))
}

func setupFormHTML(errText, defaultWorkspace string) template.HTML {
	if defaultWorkspace == "" {
		defaultWorkspace = "Acme"
	}
	errBlock := ""
	if errText != "" {
		errBlock = `<div class="mb-4 bg-red-50 border border-red-200 rounded-lg p-3 text-sm text-red-800">` + template.HTMLEscapeString(errText) + `</div>`
	}
	return template.HTML(errBlock + `<form id="setup-passkey-form" method="POST" class="space-y-4">
<div>
  <label class="block text-sm font-medium text-gray-700">Workspace name</label>
  <input class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" name="workspace_name" value="` + template.HTMLEscapeString(defaultWorkspace) + `" required autocomplete="organization">
</div>
<div>
  <label class="block text-sm font-medium text-gray-700">Tenant slug</label>
  <input class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" name="tenant_slug" placeholder="acme" required autocomplete="off">
</div>
<div>
  <label class="block text-sm font-medium text-gray-700">Owner name</label>
  <input class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" name="name" required autocomplete="name">
</div>
<div>
  <label class="block text-sm font-medium text-gray-700">Owner email</label>
  <input class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" name="email" type="email" required autocomplete="email">
</div>
<button class="w-full bg-blue-600 text-white px-6 py-2.5 rounded-lg font-medium hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 text-sm" type="submit">Create workspace and enroll passkey</button>
</form>
<p id="setup-status" class="mt-4 text-sm text-gray-600"></p>
<script>
(function() {
  const form = document.getElementById("setup-passkey-form");
  const status = document.getElementById("setup-status");
  if (!form) return;

  const b64ToBuf = (b64) => Uint8Array.from(atob((b64 + "===".slice((b64.length + 3) % 4)).replace(/-/g, "+").replace(/_/g, "/")), c => c.charCodeAt(0));
  const bufToB64 = (buf) => btoa(String.fromCharCode(...new Uint8Array(buf))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
  const normalizeCreation = (pk) => {
    pk.challenge = b64ToBuf(pk.challenge);
    pk.user.id = b64ToBuf(pk.user.id);
    if (pk.excludeCredentials) pk.excludeCredentials = pk.excludeCredentials.map(c => ({...c, id: b64ToBuf(c.id)}));
    return pk;
  };
  const marshalCreation = (cred) => ({
    id: cred.id,
    rawId: bufToB64(cred.rawId),
    type: cred.type,
    response: {
      attestationObject: bufToB64(cred.response.attestationObject),
      clientDataJSON: bufToB64(cred.response.clientDataJSON),
      transports: cred.response.getTransports ? cred.response.getTransports() : []
    }
  });

  const buildFormData = (form) => {
    const fd = new FormData(form);
    // Some browsers can visually autofill without the value being present in FormData.
    // Force-set current DOM values as a safety net.
    form.querySelectorAll("input[name],select[name],textarea[name]").forEach((el) => {
      try { fd.set(el.name, el.value || ""); } catch (_) {}
    });
    return fd;
  };

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    status.className = "mt-4 text-sm text-gray-600";
    status.textContent = "Starting setup...";
    try {
      const startResp = await fetch("/setup/passkey/start", { method: "POST", body: buildFormData(form) });
      if (!startResp.ok) throw new Error(await startResp.text());
      const start = await startResp.json();
      const opts = start.options.publicKey || (start.options.response && start.options.response.publicKey) || start.options;
      const credential = await navigator.credentials.create({ publicKey: normalizeCreation(opts) });
      const finishResp = await fetch("/setup/passkey/finish?flow_id=" + encodeURIComponent(start.flow_id), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(marshalCreation(credential))
      });
      if (!finishResp.ok) throw new Error(await finishResp.text());
      const finish = await finishResp.json();
      window.location.href = finish.redirect;
    } catch (err) {
      status.className = "mt-4 text-sm text-red-700";
      status.textContent = "Setup failed: " + err.message;
    }
  });
})();
</script>
</div>`)
}

func loginFormHTML(slug, errText string) template.HTML {
	errBlock := ""
	if errText != "" {
		errBlock = `<div class="mb-4 bg-red-50 border border-red-200 rounded-lg p-3 text-sm text-red-800">` + template.HTMLEscapeString(errText) + `</div>`
	}
	return template.HTML(errBlock + `<div class="space-y-4">
<button id="login-discoverable-btn" class="w-full bg-blue-600 text-white px-6 py-2.5 rounded-lg font-medium hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 text-sm" type="button">Use passkey</button>
<div class="flex items-center gap-3">
  <div class="h-px bg-gray-200 flex-1"></div>
  <div class="text-xs text-gray-500 font-medium">or</div>
  <div class="h-px bg-gray-200 flex-1"></div>
</div>
<form id="login-passkey-form" method="POST" class="space-y-4">
<div>
  <label class="block text-sm font-medium text-gray-700">Email</label>
  <input class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" name="email" type="email" required autocomplete="email">
</div>
<button class="w-full bg-blue-600 text-white px-6 py-2.5 rounded-lg font-medium hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 text-sm" type="submit">Sign in with passkey</button>
</form>
<p id="login-status" class="mt-4 text-sm text-gray-600"></p>
<script>
(function() {
  const form = document.getElementById("login-passkey-form");
  const discoverableBtn = document.getElementById("login-discoverable-btn");
  const status = document.getElementById("login-status");
  if (!form || !status) return;

  const b64ToBuf = (b64) => Uint8Array.from(atob((b64 + "===".slice((b64.length + 3) % 4)).replace(/-/g, "+").replace(/_/g, "/")), c => c.charCodeAt(0));
  const bufToB64 = (buf) => btoa(String.fromCharCode(...new Uint8Array(buf))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
  const normalizeAssertion = (pk) => {
    pk.challenge = b64ToBuf(pk.challenge);
    if (pk.allowCredentials) pk.allowCredentials = pk.allowCredentials.map(c => ({...c, id: b64ToBuf(c.id)}));
    return pk;
  };
  const marshalAssertion = (cred) => ({
    id: cred.id,
    rawId: bufToB64(cred.rawId),
    type: cred.type,
    response: {
      authenticatorData: bufToB64(cred.response.authenticatorData),
      clientDataJSON: bufToB64(cred.response.clientDataJSON),
      signature: bufToB64(cred.response.signature),
      userHandle: cred.response.userHandle ? bufToB64(cred.response.userHandle) : null
    }
  });

  const buildFormData = (form) => {
    const fd = new FormData(form);
    form.querySelectorAll("input[name],select[name],textarea[name]").forEach((el) => {
      try { fd.set(el.name, el.value || ""); } catch (_) {}
    });
    return fd;
  };

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    status.className = "mt-4 text-sm text-gray-600";
    status.textContent = "Starting passkey login...";
    try {
      const startResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/login/passkey/start", { method: "POST", body: buildFormData(form) });
      if (!startResp.ok) throw new Error(await startResp.text());
      const start = await startResp.json();
      const opts = start.options.publicKey || (start.options.response && start.options.response.publicKey) || start.options;
      const assertion = await navigator.credentials.get({ publicKey: normalizeAssertion(opts) });
      const finishResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/login/passkey/finish?flow_id=" + encodeURIComponent(start.flow_id), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(marshalAssertion(assertion))
      });
      if (!finishResp.ok) throw new Error(await finishResp.text());
      const finish = await finishResp.json();
      window.location.href = finish.redirect;
    } catch (err) {
      status.className = "mt-4 text-sm text-red-700";
      status.textContent = "Login failed: " + err.message;
    }
  });

  if(discoverableBtn){
    discoverableBtn.addEventListener("click", async () => {
      status.className = "mt-4 text-sm text-gray-600";
      status.textContent = "Starting passkey login...";
      try {
        const startResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/login/passkey/discoverable/start", { method: "POST" });
        if (!startResp.ok) throw new Error(await startResp.text());
        const start = await startResp.json();
        const opts = start.options.publicKey || (start.options.response && start.options.response.publicKey) || start.options;
        const assertion = await navigator.credentials.get({ publicKey: normalizeAssertion(opts) });
        const finishResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/login/passkey/discoverable/finish?flow_id=" + encodeURIComponent(start.flow_id), {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(marshalAssertion(assertion))
        });
        if (!finishResp.ok) throw new Error(await finishResp.text());
        const finish = await finishResp.json();
        window.location.href = finish.redirect;
      } catch (err) {
        status.className = "mt-4 text-sm text-red-700";
        status.textContent = "Login failed: " + err.message;
      }
    });
  }
})();
</script>`)
}

func inviteRedeemHTML(slug, email, token string) template.HTML {
	return template.HTML(`<p>You have been invited to join <code>` + template.HTMLEscapeString(slug) + `</code>.</p>
<form id="invite-passkey-form" method="POST">
<label>Email</label><input name="email" type="email" value="` + template.HTMLEscapeString(email) + `" readonly>
<label>Your name*</label><input name="name" required>
<button type="submit">Enroll passkey and join</button>
</form>
<p id="invite-status"></p>
<script>
(function() {
  const form = document.getElementById("invite-passkey-form");
  const status = document.getElementById("invite-status");
  if (!form) return;
  const token = "` + template.HTMLEscapeString(token) + `";

  const b64ToBuf = (b64) => Uint8Array.from(atob((b64 + "===".slice((b64.length + 3) % 4)).replace(/-/g, "+").replace(/_/g, "/")), c => c.charCodeAt(0));
  const bufToB64 = (buf) => btoa(String.fromCharCode(...new Uint8Array(buf))).replace(/\\+/g, "-").replace(/\\//g, "_").replace(/=+$/g, "");
  const normalizeCreation = (pk) => {
    pk.challenge = b64ToBuf(pk.challenge);
    pk.user.id = b64ToBuf(pk.user.id);
    if (pk.excludeCredentials) pk.excludeCredentials = pk.excludeCredentials.map(c => ({...c, id: b64ToBuf(c.id)}));
    return pk;
  };
  const marshalCreation = (cred) => ({
    id: cred.id,
    rawId: bufToB64(cred.rawId),
    type: cred.type,
    response: {
      attestationObject: bufToB64(cred.response.attestationObject),
      clientDataJSON: bufToB64(cred.response.clientDataJSON),
      transports: cred.response.getTransports ? cred.response.getTransports() : []
    }
  });

  const buildFormData = (form) => {
    const fd = new FormData(form);
    form.querySelectorAll("input[name],select[name],textarea[name]").forEach((el) => {
      try { fd.set(el.name, el.value || ""); } catch (_) {}
    });
    return fd;
  };

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    status.textContent = "Starting enrollment...";
    try {
      const startResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/invite/" + encodeURIComponent(token) + "/passkey/start", { method: "POST", body: buildFormData(form) });
      if (!startResp.ok) throw new Error(await startResp.text());
      const start = await startResp.json();
      const opts = start.options.publicKey || (start.options.response && start.options.response.publicKey) || start.options;
      const credential = await navigator.credentials.create({ publicKey: normalizeCreation(opts) });
      const finishResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/invite/" + encodeURIComponent(token) + "/passkey/finish?flow_id=" + encodeURIComponent(start.flow_id), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(marshalCreation(credential))
      });
      if (!finishResp.ok) throw new Error(await finishResp.text());
      const finish = await finishResp.json();
      window.location.href = finish.redirect;
    } catch (err) {
      status.textContent = "Enrollment failed: " + err.message;
    }
  });
})();
</script>`)
}
