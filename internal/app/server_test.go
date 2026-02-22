package app

import (
	"attention-crm/internal/tenantdb"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestClientIPTrustedProxy(t *testing.T) {
	s := &Server{cfg: Config{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}}

	r := httptest.NewRequest("GET", "http://example.test/t/acme/app", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.9, 127.0.0.1")

	if got := s.clientIP(r); got != "203.0.113.9" {
		t.Fatalf("clientIP trusted proxy got %q want %q", got, "203.0.113.9")
	}
}

func TestClientIPUntrustedProxyIgnoresForwarded(t *testing.T) {
	s := &Server{cfg: Config{TrustedProxies: nil}}

	r := httptest.NewRequest("GET", "http://example.test/t/acme/app", nil)
	r.RemoteAddr = "198.51.100.10:5678"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")

	if got := s.clientIP(r); got != "198.51.100.10" {
		t.Fatalf("clientIP untrusted got %q want %q", got, "198.51.100.10")
	}
}

func TestClientIPTrustedProxyFallsBackRemoteOnBadForwarded(t *testing.T) {
	s := &Server{cfg: Config{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}}

	r := httptest.NewRequest("GET", "http://example.test/t/acme/app", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "not-an-ip")
	r.Header.Set("X-Real-IP", "also-bad")

	if got := s.clientIP(r); got != "127.0.0.1" {
		t.Fatalf("clientIP bad forwarded got %q want %q", got, "127.0.0.1")
	}
}

func TestLooksLikeContactName(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{input: "Sarah", want: true},
		{input: "Mike Chen", want: true},
		{input: "O'Neill", want: true},
		{input: "Jean-Luc Picard", want: true},
		{input: "Call Sarah tomorrow", want: false},
		{input: "", want: false},
		{input: "12345", want: false},
		{input: "Sarah_J", want: false},
	}

	for _, tc := range tests {
		got := looksLikeContactName(tc.input)
		if got != tc.want {
			t.Fatalf("looksLikeContactName(%q)=%v want %v", tc.input, got, tc.want)
		}
	}
}

func TestUniversalNoteParsing(t *testing.T) {
	now := time.Date(2026, 2, 21, 10, 0, 0, 0, time.Local)
	if got, ok := parseDueSuggestionLocal("Call Sarah tomorrow", now); !ok || got == "" {
		t.Fatalf("expected due suggestion for tomorrow, got ok=%v value=%q", ok, got)
	}
	if q := extractContactQueryFromNote("Call Sarah tomorrow"); q != "sarah" {
		t.Fatalf("expected extracted query %q, got %q", "sarah", q)
	}
	if !looksLikeNote("Call Sarah tomorrow") {
		t.Fatalf("expected looksLikeNote true")
	}
	if looksLikeNote("Sarah Johnson") {
		t.Fatalf("expected looksLikeNote false for name-only input")
	}
}

func TestOmniBuildActionsLimitsToTwoPlusPick(t *testing.T) {
	now := time.Date(2026, 2, 21, 10, 0, 0, 0, time.Local)
	q := "Bob suggested that we could lower the price"
	matches := []tenantdb.Contact{
		{ID: 1, Name: "Bob Smith"},
		{ID: 2, Name: "Sarah Chen"},
		{ID: 3, Name: "Alex Johnson"},
	}

	acts := omniBuildActions(now, q, matches, nil)
	if len(acts) != 3 {
		t.Fatalf("expected 3 actions (2 log + pick), got %d: %+v", len(acts), acts)
	}
	if acts[0].Type != "log_interaction" || acts[1].Type != "log_interaction" {
		t.Fatalf("expected first two actions log_interaction, got %+v", acts)
	}
	if acts[2].Type != "pick_entity" {
		t.Fatalf("expected third action pick_entity, got %+v", acts)
	}
	if acts[0].ContactID != 1 || acts[1].ContactID != 2 {
		t.Fatalf("expected first two contacts 1 and 2, got %+v", acts)
	}
}

func TestOmniBuildActionsFallsBackToOptions(t *testing.T) {
	now := time.Date(2026, 2, 21, 10, 0, 0, 0, time.Local)
	q := "Call Bob tomorrow"
	opts := []tenantdb.Contact{
		{ID: 11, Name: "Bob Smith"},
		{ID: 12, Name: "Sarah Chen"},
	}

	acts := omniBuildActions(now, q, nil, opts)
	if len(acts) != 3 {
		t.Fatalf("expected 3 actions (2 log + pick), got %d: %+v", len(acts), acts)
	}
	if acts[0].ContactID != 11 || acts[1].ContactID != 12 {
		t.Fatalf("expected fallback to options, got %+v", acts)
	}
	if acts[2].Type != "pick_entity" {
		t.Fatalf("expected pick_entity row, got %+v", acts)
	}
	if acts[0].DueAt == "" || acts[2].DueAt == "" {
		t.Fatalf("expected due suggestion to propagate, got %+v", acts)
	}
}

func TestOmniBuildActionsCreateContact(t *testing.T) {
	now := time.Date(2026, 2, 21, 10, 0, 0, 0, time.Local)
	q := "Sarah Chen"

	acts := omniBuildActions(now, q, nil, nil)
	if len(acts) != 1 {
		t.Fatalf("expected 1 create_contact action, got %d: %+v", len(acts), acts)
	}
	if acts[0].Type != "create_contact" || acts[0].Name != "Sarah Chen" {
		t.Fatalf("unexpected action: %+v", acts[0])
	}
}
