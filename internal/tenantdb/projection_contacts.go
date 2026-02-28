package tenantdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type ContactsProjection struct{}

func (p ContactsProjection) Name() string { return "contacts" }

func (p ContactsProjection) Reset(tx *sql.Tx, workspaceID int64) error {
	_, err := tx.Exec(`DELETE FROM contacts WHERE workspace_id = ?`, workspaceID)
	return err
}

type contactCreatedPayload struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Company string `json:"company"`
	Notes   string `json:"notes"`
}

type contactFieldSetPayload struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

func (p ContactsProjection) Apply(tx *sql.Tx, workspaceID int64, ev LedgerEvent) error {
	if ev.EntityType != "contact" {
		return nil
	}
	if !ev.EntityID.Valid || ev.EntityID.Int64 <= 0 {
		return errors.New("missing entity_id")
	}
	contactID := ev.EntityID.Int64

	switch ev.Op {
	case "contact.created":
		raw := strings.TrimSpace(ev.PayloadJSON)
		if raw == "" {
			return errors.New("missing payload_json")
		}
		var payload contactCreatedPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return err
		}
		payload.Name = strings.TrimSpace(payload.Name)
		if payload.Name == "" {
			return errors.New("name required")
		}
		payload.Email = strings.ToLower(strings.TrimSpace(payload.Email))
		payload.Phone = strings.TrimSpace(payload.Phone)
		payload.Company = strings.TrimSpace(payload.Company)

		_, err := tx.Exec(`
INSERT INTO contacts(
  id, workspace_id,
  name, email, phone, company, notes,
  created_at, updated_at
) VALUES(?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  email = excluded.email,
  phone = excluded.phone,
  company = excluded.company,
  notes = excluded.notes,
  updated_at = excluded.updated_at
`, contactID, workspaceID, payload.Name, payload.Email, payload.Phone, payload.Company, payload.Notes, ev.CreatedAt, ev.CreatedAt)
		return err

	case "contact.field.set":
		raw := strings.TrimSpace(ev.PayloadJSON)
		if raw == "" {
			return errors.New("missing payload_json")
		}
		var payload contactFieldSetPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return err
		}
		field := strings.TrimSpace(strings.ToLower(payload.Field))
		value := payload.Value
		switch field {
		case "name":
			value = strings.TrimSpace(value)
			if value == "" {
				return errors.New("contact name required")
			}
		case "email":
			value = strings.ToLower(strings.TrimSpace(value))
		case "phone", "company", "notes":
			value = strings.TrimSpace(value)
		default:
			return errors.New("invalid field")
		}

		_, err := tx.Exec(`
UPDATE contacts
SET `+field+` = ?, updated_at = ?
WHERE workspace_id = ? AND id = ?
`, value, ev.CreatedAt, workspaceID, contactID)
		return err
	}

	return nil
}

