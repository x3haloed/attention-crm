package tenantdb

import (
	"database/sql"
	"errors"
	"strings"
)

func (s *Store) AllocateEntityID(entityType string) (int64, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return 0, err
	}
	entityType = strings.TrimSpace(entityType)
	if entityType == "" {
		return 0, errors.New("entity_type required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	id, err := allocateEntityIDTx(tx, workspaceID, entityType)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

func allocateEntityIDTx(tx *sql.Tx, workspaceID int64, entityType string) (int64, error) {
	if _, err := tx.Exec(`
INSERT INTO entity_id_counters(workspace_id, entity_type, next_id)
VALUES(?,?,1)
ON CONFLICT(workspace_id, entity_type) DO NOTHING
`, workspaceID, entityType); err != nil {
		return 0, err
	}

	row := tx.QueryRow(`SELECT next_id FROM entity_id_counters WHERE workspace_id = ? AND entity_type = ?`, workspaceID, entityType)
	var next int64
	if err := row.Scan(&next); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`
UPDATE entity_id_counters
SET next_id = next_id + 1
WHERE workspace_id = ? AND entity_type = ?
`, workspaceID, entityType); err != nil {
		return 0, err
	}
	return next, nil
}

