package tenantdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type OutboxProjection struct{}

func (p OutboxProjection) Name() string { return "outbox" }

func (p OutboxProjection) Reset(tx *sql.Tx, workspaceID int64) error {
	_, err := tx.Exec(`DELETE FROM outbox_effects WHERE workspace_id = ?`, workspaceID)
	return err
}

type emailCommittedPayload struct {
	ExternalEffectID string   `json:"external_effect_id"`
	To               string   `json:"to"`
	Subject          string   `json:"subject"`
	Body             []string `json:"body"`
}

func (p OutboxProjection) Apply(tx *sql.Tx, workspaceID int64, ev LedgerEvent) error {
	switch ev.Op {
	case "email.send.committed":
		if ev.EntityType != "email" {
			return nil
		}
		if !ev.EntityID.Valid || ev.EntityID.Int64 <= 0 {
			return errors.New("missing email entity_id")
		}
		raw := strings.TrimSpace(ev.PayloadJSON)
		if raw == "" {
			return errors.New("missing payload_json")
		}
		var payload emailCommittedPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return err
		}
		payload.ExternalEffectID = strings.TrimSpace(payload.ExternalEffectID)
		if payload.ExternalEffectID == "" {
			return errors.New("external_effect_id required")
		}
		kind := "email.send"

		_, err := tx.Exec(`
INSERT INTO outbox_effects(
  workspace_id, kind, status,
  email_entity_id, commit_event_id, external_effect_id,
  payload_json,
  created_at, updated_at
) VALUES(?,?,?,?,?,?,?,?,?)
ON CONFLICT(workspace_id, commit_event_id) DO NOTHING
`, workspaceID, kind, "pending", ev.EntityID.Int64, ev.ID, payload.ExternalEffectID, raw, ev.CreatedAt, ev.CreatedAt)
		return err
	}
	return nil
}

