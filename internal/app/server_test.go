package app

import (
	"testing"
	"time"
)

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
