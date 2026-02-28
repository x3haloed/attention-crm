package tenantdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type InteractionsProjection struct{}

func (p InteractionsProjection) Name() string { return "interactions" }

func (p InteractionsProjection) Reset(tx *sql.Tx, workspaceID int64) error {
	_, err := tx.Exec(`DELETE FROM interactions WHERE workspace_id = ?`, workspaceID)
	return err
}

type interactionCreatedPayload struct {
	ContactID int64  `json:"contact_id"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	DueAt     string `json:"due_at,omitempty"`
}

func (p InteractionsProjection) Apply(tx *sql.Tx, workspaceID int64, ev LedgerEvent) error {
	if ev.EntityType != "interaction" {
		return nil
	}
	if !ev.EntityID.Valid || ev.EntityID.Int64 <= 0 {
		return errors.New("missing entity_id")
	}
	interactionID := ev.EntityID.Int64

	switch ev.Op {
	case "interaction.created":
		raw := strings.TrimSpace(ev.PayloadJSON)
		if raw == "" {
			return errors.New("missing payload_json")
		}
		var payload interactionCreatedPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return err
		}
		if payload.ContactID <= 0 {
			return errors.New("contact_id required")
		}
		payload.Type = strings.TrimSpace(strings.ToLower(payload.Type))
		switch payload.Type {
		case "note", "call", "email", "meeting":
		default:
			return errors.New("invalid interaction type")
		}
		payload.Content = strings.TrimSpace(payload.Content)
		if payload.Content == "" {
			return errors.New("content required")
		}
		due := strings.TrimSpace(payload.DueAt)
		var dueAt any
		if due != "" {
			dueAt = due
		}

		var actor any
		if ev.ActorUserID.Valid && ev.ActorUserID.Int64 > 0 {
			actor = ev.ActorUserID.Int64
		}

		_, err := tx.Exec(`
INSERT INTO interactions(
  id, workspace_id,
  contact_id, type, content,
  due_at, completed_at,
  created_by_user_id, updated_by_user_id,
  created_at
) VALUES(?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET
  contact_id = excluded.contact_id,
  type = excluded.type,
  content = excluded.content,
  due_at = excluded.due_at,
  updated_by_user_id = excluded.updated_by_user_id
`, interactionID, workspaceID, payload.ContactID, payload.Type, payload.Content, dueAt, nil, actor, actor, ev.CreatedAt)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`
UPDATE contacts
SET updated_at = ?
WHERE workspace_id = ? AND id = ?
`, ev.CreatedAt, workspaceID, payload.ContactID)
		return err

	case "interaction.completed":
		// completed_at is the event's created_at, by definition.
		var actor any
		if ev.ActorUserID.Valid && ev.ActorUserID.Int64 > 0 {
			actor = ev.ActorUserID.Int64
		}
		if _, err := tx.Exec(`
UPDATE interactions
SET completed_at = ?, updated_by_user_id = COALESCE(?, updated_by_user_id)
WHERE workspace_id = ? AND id = ?
`, ev.CreatedAt, actor, workspaceID, interactionID); err != nil {
			return err
		}
		_, err := tx.Exec(`
UPDATE contacts
SET updated_at = ?
WHERE workspace_id = ? AND id = (SELECT contact_id FROM interactions WHERE workspace_id = ? AND id = ?)
`, ev.CreatedAt, workspaceID, workspaceID, interactionID)
		return err
	}

	return nil
}

