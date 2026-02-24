package tenantdb

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
)

func (s *Store) WriteContactsCSV(out io.Writer) error {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}
	rows, err := s.db.Query(`
SELECT id, name, email, phone, company, notes, created_at, updated_at
FROM contacts
WHERE workspace_id = ?
ORDER BY updated_at DESC, id DESC
`, workspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()

	w := csv.NewWriter(out)
	if err := w.Write([]string{"contact_id", "name", "email", "phone", "company", "notes", "created_at", "updated_at"}); err != nil {
		return err
	}
	for rows.Next() {
		var (
			id        int64
			name      string
			email     string
			phone     string
			company   string
			notes     string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&id, &name, &email, &phone, &company, &notes, &createdAt, &updatedAt); err != nil {
			return err
		}
		if err := w.Write([]string{
			strconv.FormatInt(id, 10),
			name,
			email,
			phone,
			company,
			notes,
			createdAt,
			updatedAt,
		}); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func (s *Store) WriteInteractionsCSV(out io.Writer) error {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
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
`, workspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()

	w := csv.NewWriter(out)
	if err := w.Write([]string{
		"interaction_id",
		"contact_id",
		"contact_name",
		"type",
		"content",
		"due_at",
		"completed_at",
		"created_by_user_id",
		"created_by",
		"updated_by_user_id",
		"updated_by",
		"created_at",
	}); err != nil {
		return err
	}

	for rows.Next() {
		var (
			id          int64
			contactID   int64
			contactName string
			typ         string
			content     string
			dueAt       sql.NullString
			completedAt sql.NullString
			createdByID sql.NullInt64
			createdBy   sql.NullString
			updatedByID sql.NullInt64
			updatedBy   sql.NullString
			createdAt   string
		)
		if err := rows.Scan(
			&id, &contactID, &contactName,
			&typ, &content, &dueAt, &completedAt,
			&createdByID, &createdBy,
			&updatedByID, &updatedBy,
			&createdAt,
		); err != nil {
			return err
		}
		if err := w.Write([]string{
			strconv.FormatInt(id, 10),
			strconv.FormatInt(contactID, 10),
			contactName,
			typ,
			content,
			nullString(dueAt),
			nullString(completedAt),
			nullInt64(createdByID),
			nullString(createdBy),
			nullInt64(updatedByID),
			nullString(updatedBy),
			createdAt,
		}); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func (s *Store) WriteDealsCSV(out io.Writer) error {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}

	rows, err := s.db.Query(`
SELECT
  d.id, d.title, d.state, d.value_cents, d.stage_label, d.next_step,
  d.next_step_due_at, d.next_step_completed_at,
  d.close_window_start, d.close_window_end,
  d.closed_at, d.closed_outcome,
  d.last_activity_at, d.created_at, d.updated_at,
  (
    SELECT group_concat(contact_id, ';') FROM (
      SELECT dc.contact_id AS contact_id
      FROM deal_contacts dc
      WHERE dc.deal_id = d.id
      ORDER BY dc.contact_id ASC
    )
  ) AS contact_ids,
  (
    SELECT group_concat(name, ';') FROM (
      SELECT c.name AS name
      FROM deal_contacts dc
      JOIN contacts c ON c.id = dc.contact_id
      WHERE dc.deal_id = d.id
      ORDER BY dc.contact_id ASC
    )
  ) AS contact_names
FROM deals d
WHERE d.workspace_id = ?
ORDER BY d.updated_at DESC, d.id DESC
`, workspaceID)
	if err != nil {
		return err
	}
	defer rows.Close()

	w := csv.NewWriter(out)
	if err := w.Write([]string{
		"deal_id",
		"title",
		"state",
		"value_cents",
		"stage_label",
		"next_step",
		"next_step_due_at",
		"next_step_completed_at",
		"close_window_start",
		"close_window_end",
		"closed_at",
		"closed_outcome",
		"last_activity_at",
		"created_at",
		"updated_at",
		"contact_ids",
		"contact_names",
	}); err != nil {
		return err
	}

	for rows.Next() {
		var (
			id                int64
			title             string
			state             string
			valueCents        sql.NullInt64
			stageLabel        string
			nextStep          string
			nextStepDueAt     sql.NullString
			nextStepCompleted sql.NullString
			closeWindowStart  sql.NullString
			closeWindowEnd    sql.NullString
			closedAt          sql.NullString
			closedOutcome     string
			lastActivityAt    string
			createdAt         string
			updatedAt         string
			contactIDs        sql.NullString
			contactNames      sql.NullString
		)
		if err := rows.Scan(
			&id, &title, &state, &valueCents, &stageLabel, &nextStep,
			&nextStepDueAt, &nextStepCompleted,
			&closeWindowStart, &closeWindowEnd,
			&closedAt, &closedOutcome,
			&lastActivityAt, &createdAt, &updatedAt,
			&contactIDs, &contactNames,
		); err != nil {
			return err
		}
		if err := w.Write([]string{
			strconv.FormatInt(id, 10),
			title,
			state,
			nullInt64(valueCents),
			stageLabel,
			nextStep,
			nullString(nextStepDueAt),
			nullString(nextStepCompleted),
			nullString(closeWindowStart),
			nullString(closeWindowEnd),
			nullString(closedAt),
			closedOutcome,
			lastActivityAt,
			createdAt,
			updatedAt,
			nullString(contactIDs),
			nullString(contactNames),
		}); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func nullString(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

func nullInt64(v sql.NullInt64) string {
	if v.Valid {
		return fmt.Sprintf("%d", v.Int64)
	}
	return ""
}

