package tenantdb

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Projection interface {
	Name() string
	Reset(tx *sql.Tx, workspaceID int64) error
	Apply(tx *sql.Tx, workspaceID int64, ev LedgerEvent) error
}

func (s *Store) RebuildProjection(p Projection) error {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}
	if p == nil {
		return errors.New("projection required")
	}
	name := strings.TrimSpace(p.Name())
	if name == "" {
		return errors.New("projection name required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := p.Reset(tx, workspaceID); err != nil {
		return err
	}
	if err := setProjectionCursor(tx, workspaceID, name, 0); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return s.ApplyProjection(p)
}

func (s *Store) ApplyProjection(p Projection) error {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}
	if p == nil {
		return errors.New("projection required")
	}
	name := strings.TrimSpace(p.Name())
	if name == "" {
		return errors.New("projection name required")
	}

	cursor, err := getProjectionCursor(s.db, workspaceID, name)
	if err != nil {
		return err
	}

	const batch = 250
	for {
		events, err := listLedgerEventsAfterID(s.db, workspaceID, cursor, batch)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}

		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		ok := false
		defer func() {
			if !ok {
				_ = tx.Rollback()
			}
		}()

		for _, ev := range events {
			if err := p.Apply(tx, workspaceID, ev); err != nil {
				return fmt.Errorf("apply projection %s to event %d (%s/%s): %w", name, ev.ID, ev.EntityType, ev.Op, err)
			}
			cursor = ev.ID
		}

		if err := setProjectionCursor(tx, workspaceID, name, cursor); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		ok = true
	}
}

func getProjectionCursor(db *sql.DB, workspaceID int64, name string) (int64, error) {
	row := db.QueryRow(`SELECT last_event_id FROM projection_cursors WHERE workspace_id = ? AND projection_name = ?`, workspaceID, name)
	var id int64
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return id, nil
}

func setProjectionCursor(tx *sql.Tx, workspaceID int64, name string, lastEventID int64) error {
	_, err := tx.Exec(`
INSERT INTO projection_cursors(workspace_id, projection_name, last_event_id)
VALUES(?,?,?)
ON CONFLICT(workspace_id, projection_name) DO UPDATE SET
  last_event_id = excluded.last_event_id,
  updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
`, workspaceID, name, lastEventID)
	return err
}

func listLedgerEventsAfterID(db *sql.DB, workspaceID int64, afterID int64, limit int) ([]LedgerEvent, error) {
	if limit <= 0 || limit > 5000 {
		limit = 250
	}
	rows, err := db.Query(`
SELECT
  id, event_version,
  actor_kind, actor_user_id,
  op, entity_type, entity_id,
  payload_json, reason, evidence_json,
  caused_by_event_id, replaces_event_id, inverse_of_event_id,
  idempotency_key,
  created_at
FROM ledger_events
WHERE workspace_id = ? AND id > ?
ORDER BY id ASC
LIMIT ?
`, workspaceID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LedgerEvent
	for rows.Next() {
		var ev LedgerEvent
		if err := rows.Scan(
			&ev.ID, &ev.EventVersion,
			&ev.ActorKind, &ev.ActorUserID,
			&ev.Op, &ev.EntityType, &ev.EntityID,
			&ev.PayloadJSON, &ev.Reason, &ev.EvidenceJSON,
			&ev.CausedByEventID, &ev.ReplacesEventID, &ev.InverseOfEventID,
			&ev.IdempotencyKey,
			&ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

