package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Server) writeSession(w http.ResponseWriter, r *http.Request, sess session) error {
	payload := sess.TenantSlug + "|" + strconv.FormatInt(sess.UserID, 10)
	sig := sign(payload, s.sessionKey)
	raw := payload + "|" + sig
	cookie := &http.Cookie{
		Name:     "attention_session",
		Value:    base64.RawURLEncoding.EncodeToString([]byte(raw)),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isSecureRequest(r),
		Path:     "/",
		Expires:  time.Now().Add(24 * time.Hour),
	}
	http.SetCookie(w, cookie)
	s.ensureCSRFCookie(w, r)
	return nil
}

func (s *Server) readSession(r *http.Request) (session, bool) {
	cookie, err := r.Cookie("attention_session")
	if err != nil {
		if s.cfg.DevNoAuth {
			if slug, _, ok := parseTenantPath(r.URL.Path); ok {
				return session{TenantSlug: slug, UserID: 1}, true
			}
		}
		return session{}, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil {
		if s.cfg.DevNoAuth {
			if slug, _, ok := parseTenantPath(r.URL.Path); ok {
				return session{TenantSlug: slug, UserID: 1}, true
			}
		}
		return session{}, false
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 {
		if s.cfg.DevNoAuth {
			if slug, _, ok := parseTenantPath(r.URL.Path); ok {
				return session{TenantSlug: slug, UserID: 1}, true
			}
		}
		return session{}, false
	}
	payload := parts[0] + "|" + parts[1]
	if !hmac.Equal([]byte(sign(payload, s.sessionKey)), []byte(parts[2])) {
		if s.cfg.DevNoAuth {
			if slug, _, ok := parseTenantPath(r.URL.Path); ok {
				return session{TenantSlug: slug, UserID: 1}, true
			}
		}
		return session{}, false
	}
	userID, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		if s.cfg.DevNoAuth {
			if slug, _, ok := parseTenantPath(r.URL.Path); ok {
				return session{TenantSlug: slug, UserID: 1}, true
			}
		}
		return session{}, false
	}
	return session{TenantSlug: parts[0], UserID: userID}, true
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
