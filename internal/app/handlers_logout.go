package app

import (
	"attention-crm/internal/control"
	"net/http"
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
