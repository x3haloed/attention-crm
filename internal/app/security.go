package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"golang.org/x/time/rate"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var cryptoRandRead = rand.Read

func (s *Server) bodyLimitMiddleware(next http.Handler) http.Handler {
	maxBytes := s.cfg.MaxRequestBodyBytes
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Safe defaults that don't interfere with inline scripts/styles.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		// Use CSP only for clickjacking protection to avoid breaking the app (inline scripts, Google Fonts, etc.).
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")

		if s.isSecureRequest(r) {
			// Only send HSTS over HTTPS.
			w.Header().Set("Strict-Transport-Security", "max-age=15552000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}

	if !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return false
	}
	if len(s.cfg.TrustedProxies) == 0 {
		return false
	}

	remote := r.RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		remote = host
	}
	remoteAddr, err := netip.ParseAddr(remote)
	if err != nil {
		return false
	}
	for _, pfx := range s.cfg.TrustedProxies {
		if pfx.Contains(remoteAddr) {
			return true
		}
	}
	return false
}

func (s *Server) clientIP(r *http.Request) string {
	remote := r.RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		remote = host
	}

	remoteAddr, err := netip.ParseAddr(remote)
	if err != nil {
		return remote
	}

	trusted := false
	for _, pfx := range s.cfg.TrustedProxies {
		if pfx.Contains(remoteAddr) {
			trusted = true
			break
		}
	}
	if !trusted {
		return remote
	}

	// If we're behind a trusted proxy, honor forwarded client IP headers.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if first != "" {
			if ip, err := netip.ParseAddr(first); err == nil {
				return ip.String()
			}
		}
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		if ip, err := netip.ParseAddr(xrip); err == nil {
			return ip.String()
		}
	}

	return remote
}

func (s *Server) allowRate(r *http.Request, bucket string, perSecond float64, burst int) bool {
	ip := s.clientIP(r)
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

func randomTokenB64(n int) (string, error) {
	if n <= 0 {
		return "", errors.New("token size must be positive")
	}
	buf := make([]byte, n)
	if _, err := cryptoRandRead(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *Server) ensureCSRFCookie(w http.ResponseWriter, r *http.Request) (string, error) {
	if c, err := r.Cookie("attention_csrf"); err == nil && c.Value != "" {
		return c.Value, nil
	}
	token, err := randomTokenB64(32)
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "attention_csrf",
		Value:    token,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isSecureRequest(r),
		Expires:  time.Now().Add(24 * time.Hour),
	})
	return token, nil
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

func requireSameOrigin(w http.ResponseWriter, r *http.Request) bool {
	// For pre-auth flows (like initial setup) we can't rely on a per-session CSRF token.
	// Instead, enforce same-origin using browser-provided headers.
	if sfs := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); sfs == "same-origin" || sfs == "same-site" {
		return true
	}
	ref := r.Header.Get("Origin")
	if ref == "" {
		ref = r.Header.Get("Referer")
	}
	u, err := url.Parse(ref)
	if err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host) {
		return true
	}
	http.Error(w, "forbidden", http.StatusForbidden)
	return false
}
