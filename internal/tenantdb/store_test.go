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
	if err := store.CreateInteractionBy(1, contactID, "note", "Called and left a message", &due); err != nil {
		t.Fatalf("CreateInteraction: %v", err)
	}

	after, err := store.ContactByID(contactID)
	if err != nil {
		t.Fatalf("ContactByID after: %v", err)
	}
	if after.UpdatedAt == before.UpdatedAt {
		t.Fatalf("expected updated_at to change; still %q", after.UpdatedAt)
	}

	timeline, err := store.ListInteractionsByContact(contactID, 10)
	if err != nil || len(timeline) == 0 {
		t.Fatalf("ListInteractionsByContact: err=%v len=%d", err, len(timeline))
	}
	if !timeline[0].CreatedBy.Valid || timeline[0].CreatedBy.String != "Owner" {
		t.Fatalf("expected interaction CreatedBy=Owner; got %+v", timeline[0].CreatedBy)
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

func TestDealsCRUDNextStepCloseAndEvents(t *testing.T) {
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
	if err := store.CreateContact("Bob Smith", "bob@example.com", "", "Acme", ""); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	contacts, err := store.SearchContacts("Bob", 10)
	if err != nil || len(contacts) != 1 {
		t.Fatalf("SearchContacts: err=%v len=%d", err, len(contacts))
	}
	contactID := contacts[0].ID

	if _, err := store.CreateDeal("New deal", nil); err == nil {
		t.Fatalf("expected CreateDeal to require at least one contact")
	}

	dealID, err := store.CreateDeal("Website redesign", []int64{contactID})
	if err != nil {
		t.Fatalf("CreateDeal: %v", err)
	}

	deal, dealContacts, err := store.DealByID(dealID)
	if err != nil {
		t.Fatalf("DealByID: %v", err)
	}
	if deal.Title != "Website redesign" || deal.State != "open" {
		t.Fatalf("unexpected deal: %+v", deal)
	}
	if len(dealContacts) != 1 || dealContacts[0] != contactID {
		t.Fatalf("unexpected deal contacts: %+v", dealContacts)
	}

	due := time.Now().Add(4 * time.Hour)
	if _, err := store.UpdateDealNextStep(dealID, "Send proposal", &due); err != nil {
		t.Fatalf("UpdateDealNextStep: %v", err)
	}
	afterNext, _, err := store.DealByID(dealID)
	if err != nil {
		t.Fatalf("DealByID after next step: %v", err)
	}
	if afterNext.NextStep != "Send proposal" || !afterNext.NextStepDueAt.Valid {
		t.Fatalf("unexpected next step state: %+v", afterNext)
	}

	if _, err := store.CompleteDealNextStep(dealID); err != nil {
		t.Fatalf("CompleteDealNextStep: %v", err)
	}
	afterComplete, _, err := store.DealByID(dealID)
	if err != nil {
		t.Fatalf("DealByID after complete: %v", err)
	}
	if !afterComplete.NextStepCompleted.Valid {
		t.Fatalf("expected next_step_completed_at to be set")
	}

	// Seed last_activity_at so we can verify events bump it.
	if _, err := store.db.Exec(`UPDATE deals SET last_activity_at = '2000-01-01T00:00:00Z' WHERE id = ?`, dealID); err != nil {
		t.Fatalf("seed last_activity_at: %v", err)
	}
	if err := store.CreateDealEventBy(1, dealID, "note", "Talked budget"); err != nil {
		t.Fatalf("CreateDealEvent: %v", err)
	}
	afterEvent, _, err := store.DealByID(dealID)
	if err != nil {
		t.Fatalf("DealByID after event: %v", err)
	}
	if afterEvent.LastActivityAt == "2000-01-01T00:00:00Z" {
		t.Fatalf("expected last_activity_at to change")
	}
	events, err := store.ListDealEvents(dealID, 10)
	if err != nil || len(events) == 0 {
		t.Fatalf("ListDealEvents: err=%v len=%d", err, len(events))
	}
	if !events[0].CreatedBy.Valid || events[0].CreatedBy.String != "Owner" {
		t.Fatalf("expected deal event CreatedBy=Owner; got %+v", events[0].CreatedBy)
	}

	if _, err := store.CloseDeal(dealID, "won", "Signed the contract"); err != nil {
		t.Fatalf("CloseDeal: %v", err)
	}
	closed, _, err := store.DealByID(dealID)
	if err != nil {
		t.Fatalf("DealByID after close: %v", err)
	}
	if closed.State != "won" || !closed.ClosedAt.Valid || closed.ClosedOutcome != "Signed the contract" {
		t.Fatalf("unexpected closed deal: %+v", closed)
	}

	listByContact, err := store.ListDealsByContact(contactID, 10)
	if err != nil || len(listByContact) == 0 {
		t.Fatalf("ListDealsByContact: err=%v len=%d", err, len(listByContact))
	}
}
