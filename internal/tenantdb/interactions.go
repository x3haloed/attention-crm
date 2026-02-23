package tenantdb

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *Store) CreateInteraction(contactID int64, interactionType, content string, dueAt *time.Time) error {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}
	interactionType = strings.TrimSpace(strings.ToLower(interactionType))
	switch interactionType {
	case "note", "call", "email", "meeting":
	default:
		return errors.New("invalid interaction type")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("content required")
	}

	var dueAtStr any
	if dueAt != nil {
		dueAtStr = dueAt.UTC().Format(time.RFC3339)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
INSERT INTO interactions(workspace_id, contact_id, type, content, due_at)
VALUES(?,?,?,?,?)
`, workspaceID, contactID, interactionType, content, dueAtStr)
	if err != nil {
		return err
	}

	// Touch contact updated_at so recency-based UIs behave like a real CRM.
	if _, err := tx.Exec(`
UPDATE contacts
SET updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ? AND workspace_id = ?
`, contactID, workspaceID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) ListRecentInteractions(limit int) ([]Interaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT i.id, i.contact_id, c.name, i.type, i.content, i.due_at, i.completed_at, i.created_at
FROM interactions i
JOIN contacts c ON c.id = i.contact_id
WHERE i.workspace_id = ?
ORDER BY i.created_at DESC, i.id DESC
LIMIT ?
`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Interaction
	for rows.Next() {
		var it Interaction
		if err := rows.Scan(&it.ID, &it.ContactID, &it.ContactName, &it.Type, &it.Content, &it.DueAt, &it.CompletedAt, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) ListNeedsAttention(limit int) ([]Interaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
SELECT i.id, i.contact_id, c.name, i.type, i.content, i.due_at, i.completed_at, i.created_at
FROM interactions i
JOIN contacts c ON c.id = i.contact_id
WHERE i.workspace_id = ?
  AND i.due_at IS NOT NULL
  AND i.completed_at IS NULL
ORDER BY i.due_at ASC, i.id ASC
LIMIT ?
`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Interaction
	for rows.Next() {
		var it Interaction
		if err := rows.Scan(&it.ID, &it.ContactID, &it.ContactName, &it.Type, &it.Content, &it.DueAt, &it.CompletedAt, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) MarkInteractionComplete(interactionID int64) error {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var contactID int64
	row := tx.QueryRow(`SELECT contact_id FROM interactions WHERE id = ? AND workspace_id = ?`, interactionID, workspaceID)
	if err := row.Scan(&contactID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("interaction not found or already completed")
		}
		return err
	}

	res, err := tx.Exec(`
UPDATE interactions
SET completed_at = ?
WHERE id = ? AND workspace_id = ? AND completed_at IS NULL
`, time.Now().UTC().Format(time.RFC3339), interactionID, workspaceID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("interaction not found or already completed")
	}

	if _, err := tx.Exec(`
UPDATE contacts
SET updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE id = ? AND workspace_id = ?
`, contactID, workspaceID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) ListInteractionsByContact(contactID int64, limit int) ([]Interaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT i.id, i.contact_id, c.name, i.type, i.content, i.due_at, i.completed_at, i.created_at
FROM interactions i
JOIN contacts c ON c.id = i.contact_id
WHERE i.workspace_id = ? AND i.contact_id = ?
ORDER BY i.created_at DESC, i.id DESC
LIMIT ?
`, workspaceID, contactID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Interaction
	for rows.Next() {
		var it Interaction
		if err := rows.Scan(&it.ID, &it.ContactID, &it.ContactName, &it.Type, &it.Content, &it.DueAt, &it.CompletedAt, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
