package tenantdb

import (
	"bytes"
	"encoding/csv"
	"path/filepath"
	"testing"
)

func TestWriteContactsCSV(t *testing.T) {
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
	if err := store.CreateContact("Sarah, Chen", "sarah@example.com", "", "Acme", "Line1\nLine2"); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	var buf bytes.Buffer
	if err := store.WriteContactsCSV(&buf); err != nil {
		t.Fatalf("WriteContactsCSV: %v", err)
	}
	r := csv.NewReader(bytes.NewReader(buf.Bytes()))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected header + at least 1 row; got %d records", len(records))
	}
	if got := records[0][0]; got != "contact_id" {
		t.Fatalf("unexpected header[0]=%q", got)
	}
	if got := records[1][1]; got != "Sarah, Chen" {
		t.Fatalf("unexpected name=%q", got)
	}
	if got := records[1][5]; got != "Line1\nLine2" {
		t.Fatalf("unexpected notes=%q", got)
	}
}

func TestWriteDealsCSVIncludesContacts(t *testing.T) {
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
	if err := store.CreateContact("Sarah", "sarah@example.com", "", "Acme", ""); err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	contacts, err := store.SearchContacts("Sarah", 10)
	if err != nil || len(contacts) != 1 {
		t.Fatalf("SearchContacts: err=%v len=%d", err, len(contacts))
	}
	dealID, err := store.CreateDeal("Website redesign", []int64{contacts[0].ID})
	if err != nil || dealID <= 0 {
		t.Fatalf("CreateDeal: err=%v id=%d", err, dealID)
	}

	var buf bytes.Buffer
	if err := store.WriteDealsCSV(&buf); err != nil {
		t.Fatalf("WriteDealsCSV: %v", err)
	}
	r := csv.NewReader(bytes.NewReader(buf.Bytes()))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("expected header + at least 1 row; got %d records", len(records))
	}
	// contact_names column is last.
	if got := records[1][len(records[1])-1]; got != "Sarah" {
		t.Fatalf("unexpected contact_names=%q", got)
	}
}

