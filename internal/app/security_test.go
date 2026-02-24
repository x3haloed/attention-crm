package app

import (
	"errors"
	"net/http/httptest"
	"testing"
)

func TestEnsureCSRFCookieFailsOnRandError(t *testing.T) {
	orig := cryptoRandRead
	defer func() { cryptoRandRead = orig }()
	cryptoRandRead = func(p []byte) (int, error) { return 0, errors.New("no entropy") }

	s := &Server{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/t/acme/app", nil)

	token, err := s.ensureCSRFCookie(rr, req)
	if err == nil {
		t.Fatalf("expected error; token=%q", token)
	}
	if got := rr.Header().Get("Set-Cookie"); got != "" {
		t.Fatalf("expected no Set-Cookie header; got %q", got)
	}
}

func TestStoreFlowFailsOnRandError(t *testing.T) {
	orig := cryptoRandRead
	defer func() { cryptoRandRead = orig }()
	cryptoRandRead = func(p []byte) (int, error) { return 0, errors.New("no entropy") }

	s := &Server{webauthnFlow: map[string]ceremonyFlow{}}
	if _, err := s.storeFlow(ceremonyFlow{}); err == nil {
		t.Fatalf("expected error")
	}
}
