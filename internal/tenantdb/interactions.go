package tenantdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func (s *Store) CreateInteraction(contactID int64, interactionType, content string, dueAt *time.Time) error {
	return s.CreateInteractionBy(0, contactID, interactionType, content, dueAt)
}

func (s *Store) CreateInteractionBy(actorUserID int64, contactID int64, interactionType, content string, dueAt *time.Time) error {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}
	// Ensure contact exists in current projection.
	if _, err := s.ContactByID(contactID); err != nil {
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

	var dueAtStr string
	if dueAt != nil {
		dueAtStr = dueAt.UTC().Format(time.RFC3339)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	interactionID, err := allocateEntityIDTx(tx, workspaceID, "interaction")
	if err != nil {
		return err
	}

	payload, _ := json.Marshal(interactionCreatedPayload{
		ContactID: contactID,
		Type:      interactionType,
		Content:   content,
		DueAt:     strings.TrimSpace(dueAtStr),
	})

	createdAt := time.Now().UTC()
	var actor any
	if actorUserID > 0 {
		actor = actorUserID
	}
	if _, err := appendLedgerEventExec(
		tx,
		workspaceID,
		1,
		createdAt.Format(time.RFC3339Nano),
		ActorKindHuman,
		actor,
		"interaction.created",
		"interaction",
		interactionID,
		string(payload),
		"",
		"",
		nil, nil, nil,
		nil,
	); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return s.ApplyProjection(InteractionsProjection{})
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
SELECT
  i.id, i.contact_id, c.name,
  i.type, i.content, i.due_at, i.completed_at,
  i.created_by_user_id, uc.name,
  i.updated_by_user_id, uu.name,
  i.created_at
FROM interactions i
JOIN contacts c ON c.id = i.contact_id
LEFT JOIN users uc ON uc.id = i.created_by_user_id
LEFT JOIN users uu ON uu.id = i.updated_by_user_id
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
		if err := rows.Scan(
			&it.ID, &it.ContactID, &it.ContactName,
			&it.Type, &it.Content, &it.DueAt, &it.CompletedAt,
			&it.CreatedByID, &it.CreatedBy,
			&it.UpdatedByID, &it.UpdatedBy,
			&it.CreatedAt,
		); err != nil {
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
SELECT
  i.id, i.contact_id, c.name,
  i.type, i.content, i.due_at, i.completed_at,
  i.created_by_user_id, uc.name,
  i.updated_by_user_id, uu.name,
  i.created_at
FROM interactions i
JOIN contacts c ON c.id = i.contact_id
LEFT JOIN users uc ON uc.id = i.created_by_user_id
LEFT JOIN users uu ON uu.id = i.updated_by_user_id
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
		if err := rows.Scan(
			&it.ID, &it.ContactID, &it.ContactName,
			&it.Type, &it.Content, &it.DueAt, &it.CompletedAt,
			&it.CreatedByID, &it.CreatedBy,
			&it.UpdatedByID, &it.UpdatedBy,
			&it.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) MarkInteractionComplete(interactionID int64) error {
	return s.MarkInteractionCompleteBy(0, interactionID)
}

func (s *Store) MarkInteractionCompleteBy(actorUserID int64, interactionID int64) error {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}
	// Validate it exists in the current projection.
	if err := s.db.QueryRow(`SELECT id FROM interactions WHERE id = ? AND workspace_id = ? AND completed_at IS NULL`, interactionID, workspaceID).Scan(new(int64)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("interaction not found or already completed")
		}
		return err
	}

	createdAt := time.Now().UTC()
	var entityID = interactionID
	actorKind := ActorKindHuman
	var actorUser any
	if actorUserID > 0 {
		actorUser = actorUserID
	}
	_, err = appendLedgerEventExec(
		s.db,
		workspaceID,
		1,
		createdAt.Format(time.RFC3339Nano),
		actorKind,
		actorUser,
		"interaction.completed",
		"interaction",
		entityID,
		`{}`,
		"",
		"",
		nil, nil, nil,
		nil,
	)
	if err != nil {
		return err
	}
	return s.ApplyProjection(InteractionsProjection{})
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
SELECT
  i.id, i.contact_id, c.name,
  i.type, i.content, i.due_at, i.completed_at,
  i.created_by_user_id, uc.name,
  i.updated_by_user_id, uu.name,
  i.created_at
FROM interactions i
JOIN contacts c ON c.id = i.contact_id
LEFT JOIN users uc ON uc.id = i.created_by_user_id
LEFT JOIN users uu ON uu.id = i.updated_by_user_id
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
		if err := rows.Scan(
			&it.ID, &it.ContactID, &it.ContactName,
			&it.Type, &it.Content, &it.DueAt, &it.CompletedAt,
			&it.CreatedByID, &it.CreatedBy,
			&it.UpdatedByID, &it.UpdatedBy,
			&it.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) LatestInteractionIDByContact(contactID int64) (int64, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return 0, err
	}
	row := s.db.QueryRow(`
SELECT id
FROM interactions
WHERE workspace_id = ? AND contact_id = ?
ORDER BY created_at DESC, id DESC
LIMIT 1
`, workspaceID, contactID)
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}
