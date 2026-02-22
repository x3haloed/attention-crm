package tenantdb

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCreateInteractionTouchesContactUpdatedAt(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tenant.sqlite")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	if err := store.CreateInitialUser("Acme", "owner@example.com", "Owner", "passwordpassword"); err != nil {
		t.Fatalf("CreateInitialUser: %v", err)
	}
	if err := store.CreateContact("Sarah Johnson", "", "", "", ""); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	contacts, err := store.SearchContacts("Sarah", 10)
	if err != nil || len(contacts) != 1 {
		t.Fatalf("SearchContacts: err=%v len=%d", err, len(contacts))
	}
	contactID := contacts[0].ID

	// Force updated_at to an old value so the test is deterministic.
	if _, err := store.db.Exec(`UPDATE contacts SET updated_at = '2000-01-01T00:00:00Z' WHERE id = ?`, contactID); err != nil {
		t.Fatalf("seed updated_at: %v", err)
	}

	before, err := store.ContactByID(contactID)
	if err != nil {
		t.Fatalf("ContactByID before: %v", err)
	}
	if before.UpdatedAt != "2000-01-01T00:00:00Z" {
		t.Fatalf("unexpected before updated_at: %q", before.UpdatedAt)
	}

	due := time.Now().Add(2 * time.Hour)
	if err := store.CreateInteraction(contactID, "note", "Called and left a message", &due); err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}

	after, err := store.ContactByID(contactID)
	if err != nil {
		t.Fatalf("ContactByID after: %v", err)
	}
	if after.UpdatedAt == before.UpdatedAt {
		t.Fatalf("expected updated_at to change; still %q", after.UpdatedAt)
	}
}

func TestDuplicateCandidates(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tenant.sqlite")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	if err := store.CreateInitialUser("Acme", "owner@example.com", "Owner", "passwordpassword"); err != nil {
		t.Fatalf("CreateInitialUser: %v", err)
	}

	if err := store.CreateContact("Sarah Johnson", "sarah@example.com", "(555) 123-4567", "Acme", ""); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if err := store.CreateContact("Sara Jonson", "", "5551234567", "Acme LLC", ""); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	dups, err := store.DuplicateCandidates("Sarah J", "sarah@example.com", "", "", 10)
	if err != nil {
		t.Fatalf("DuplicateCandidates: %v", err)
	}
	if len(dups) == 0 {
		t.Fatalf("expected at least one duplicate")
	}
	foundEmail := false
	for _, d := range dups {
		if d.Reason == "email match" || d.Reason == "email match, name/company fuzzy" || d.Reason == "name/company fuzzy, email match" {
			foundEmail = true
		}
	}
	if !foundEmail {
		t.Fatalf("expected email match reason in %+v", dups)
	}

	dupsPhone, err := store.DuplicateCandidates("", "", "555-123-4567", "", 10)
	if err != nil {
		t.Fatalf("DuplicateCandidates phone: %v", err)
	}
	if len(dupsPhone) == 0 {
		t.Fatalf("expected phone duplicates")
	}
}

func TestSearchContactsFTSRankingBoostsName(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tenant.sqlite")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	if err := store.CreateInitialUser("Acme", "owner@example.com", "Owner", "passwordpassword"); err != nil {
		t.Fatalf("CreateInitialUser: %v", err)
	}

	// If ranking is sane, searching "sarah" should prefer the name match over a notes-only match.
	if err := store.CreateContact("Sarah Johnson", "", "", "", ""); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if err := store.CreateContact("Random Person", "", "", "", "met sarah at conference"); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	results, err := store.SearchContacts("sarah", 10)
	if err != nil {
		t.Fatalf("SearchContacts: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2+ results, got %d", len(results))
	}
	if results[0].Name != "Sarah Johnson" {
		t.Fatalf("expected top result to be name match, got %q", results[0].Name)
	}
}

func TestUpdateContactFieldTouchesUpdatedAt(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tenant.sqlite")

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()

	if err := store.CreateInitialUser("Acme", "owner@example.com", "Owner", "passwordpassword"); err != nil {
		t.Fatalf("CreateInitialUser: %v", err)
	}
	if err := store.CreateContact("Sarah Johnson", "", "", "", ""); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	contacts, err := store.SearchContacts("Sarah", 10)
	if err != nil || len(contacts) != 1 {
		t.Fatalf("SearchContacts: err=%v len=%d", err, len(contacts))
	}
	contactID := contacts[0].ID

	if _, err := store.db.Exec(`UPDATE contacts SET updated_at = '2000-01-01T00:00:00Z' WHERE id = ?`, contactID); err != nil {
		t.Fatalf("seed updated_at: %v", err)
	}

	updatedAt, err := store.UpdateContactField(contactID, "email", "SARAH@EXAMPLE.COM")
	if err != nil {
		t.Fatalf("UpdateContactField: %v", err)
	}
	if updatedAt == "2000-01-01T00:00:00Z" {
		t.Fatalf("expected updated_at to change")
	}

	after, err := store.ContactByID(contactID)
	if err != nil {
		t.Fatalf("ContactByID: %v", err)
	}
	if after.Email != "sarah@example.com" {
		t.Fatalf("expected email to be normalized, got %q", after.Email)
	}
	if after.UpdatedAt == "2000-01-01T00:00:00Z" {
		t.Fatalf("expected updated_at to change, got %q", after.UpdatedAt)
	}
}
