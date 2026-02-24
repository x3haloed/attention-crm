package tenantdb

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *Store) ListDealsNeedsAttention(limit int) ([]Deal, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	cutoff := now.Add(48 * time.Hour).Format(time.RFC3339Nano)
	staleCutoff := now.Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)

	rows, err := s.db.Query(`
SELECT id, title, state, value_cents, stage_label, next_step, next_step_due_at, next_step_completed_at,
       close_window_start, close_window_end, closed_at, closed_outcome, last_activity_at, created_at, updated_at
FROM deals
WHERE workspace_id = ?
  AND state = 'open'
  AND (
    TRIM(COALESCE(next_step, '')) = ''
    OR (next_step_due_at IS NOT NULL AND next_step_due_at != '' AND next_step_completed_at IS NULL AND next_step_due_at <= ?)
    OR (last_activity_at IS NOT NULL AND last_activity_at != '' AND last_activity_at <= ?)
  )
ORDER BY
  CASE WHEN TRIM(COALESCE(next_step, '')) = '' THEN 0 ELSE 1 END ASC,
  COALESCE(next_step_due_at, '') ASC,
  last_activity_at DESC,
  id DESC
LIMIT ?
`, workspaceID, cutoff, staleCutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Deal
	for rows.Next() {
		var d Deal
		if err := rows.Scan(
			&d.ID,
			&d.Title,
			&d.State,
			&d.ValueCents,
			&d.StageLabel,
			&d.NextStep,
			&d.NextStepDueAt,
			&d.NextStepCompleted,
			&d.CloseWindowStart,
			&d.CloseWindowEnd,
			&d.ClosedAt,
			&d.ClosedOutcome,
			&d.LastActivityAt,
			&d.CreatedAt,
			&d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListDealsPipeline(limit int) ([]DealPipelineRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	cutoff := now.Add(48 * time.Hour).Format(time.RFC3339Nano)
	staleCutoff := now.Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)

	rows, err := s.db.Query(`
SELECT
  d.id, d.title, d.state, d.value_cents, d.stage_label, d.next_step, d.next_step_due_at, d.next_step_completed_at,
  d.close_window_start, d.close_window_end, d.closed_at, d.closed_outcome, d.last_activity_at, d.created_at, d.updated_at,
  MIN(c.id) AS primary_contact_id,
  MIN(c.name) AS primary_contact_name,
  MIN(c.company) AS primary_contact_company,
  COUNT(dc.contact_id) AS contact_count
FROM deals d
JOIN deal_contacts dc ON dc.deal_id = d.id
JOIN contacts c ON c.id = dc.contact_id
WHERE d.workspace_id = ?
  AND d.state = 'open'
GROUP BY d.id
ORDER BY
  CASE WHEN TRIM(COALESCE(d.next_step, '')) = '' THEN 0 ELSE 1 END ASC,
  CASE
    WHEN d.next_step_due_at IS NOT NULL AND d.next_step_due_at != '' AND d.next_step_completed_at IS NULL AND d.next_step_due_at <= ? THEN 0
    ELSE 1
  END ASC,
  CASE
    WHEN d.last_activity_at IS NOT NULL AND d.last_activity_at != '' AND d.last_activity_at <= ? THEN 0
    ELSE 1
  END ASC,
  COALESCE(d.next_step_due_at, '') ASC,
  d.last_activity_at DESC,
  d.id DESC
LIMIT ?
`, workspaceID, cutoff, staleCutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DealPipelineRow
	for rows.Next() {
		var row DealPipelineRow
		if err := rows.Scan(
			&row.ID,
			&row.Title,
			&row.State,
			&row.ValueCents,
			&row.StageLabel,
			&row.NextStep,
			&row.NextStepDueAt,
			&row.NextStepCompleted,
			&row.CloseWindowStart,
			&row.CloseWindowEnd,
			&row.ClosedAt,
			&row.ClosedOutcome,
			&row.LastActivityAt,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.PrimaryContactID,
			&row.PrimaryContactName,
			&row.PrimaryContactCompany,
			&row.ContactCount,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) CreateDeal(title string, contactIDs []int64) (int64, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0, errors.New("deal title required")
	}
	if len(contactIDs) == 0 {
		return 0, errors.New("deal must attach to at least one contact")
	}

	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO deals(workspace_id, title, state, last_activity_at, created_at, updated_at) VALUES(?, ?, 'open', ?, ?, ?)`,
		workspaceID, title, now, now, now,
	)
	if err != nil {
		return 0, err
	}
	dealID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	seen := map[int64]bool{}
	for _, cid := range contactIDs {
		if cid <= 0 || seen[cid] {
			continue
		}
		seen[cid] = true
		if _, err := tx.Exec(`INSERT INTO deal_contacts(deal_id, contact_id) VALUES(?, ?)`, dealID, cid); err != nil {
			return 0, err
		}
	}
	if len(seen) == 0 {
		return 0, errors.New("deal must attach to at least one contact")
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return dealID, nil
}

func (s *Store) DealByID(dealID int64) (Deal, []int64, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return Deal{}, nil, err
	}
	row := s.db.QueryRow(`
SELECT id, title, state, value_cents, stage_label, next_step, next_step_due_at, next_step_completed_at,
       close_window_start, close_window_end, closed_at, closed_outcome, last_activity_at, created_at, updated_at
FROM deals
WHERE workspace_id = ? AND id = ?
`, workspaceID, dealID)
	var d Deal
	if err := row.Scan(
		&d.ID,
		&d.Title,
		&d.State,
		&d.ValueCents,
		&d.StageLabel,
		&d.NextStep,
		&d.NextStepDueAt,
		&d.NextStepCompleted,
		&d.CloseWindowStart,
		&d.CloseWindowEnd,
		&d.ClosedAt,
		&d.ClosedOutcome,
		&d.LastActivityAt,
		&d.CreatedAt,
		&d.UpdatedAt,
	); err != nil {
		return Deal{}, nil, err
	}

	rows, err := s.db.Query(`SELECT contact_id FROM deal_contacts WHERE deal_id = ? ORDER BY contact_id`, dealID)
	if err != nil {
		return Deal{}, nil, err
	}
	defer rows.Close()
	var contactIDs []int64
	for rows.Next() {
		var cid int64
		if err := rows.Scan(&cid); err != nil {
			return Deal{}, nil, err
		}
		contactIDs = append(contactIDs, cid)
	}
	if err := rows.Err(); err != nil {
		return Deal{}, nil, err
	}
	return d, contactIDs, nil
}

func (s *Store) ListDealsByContact(contactID int64, limit int) ([]Deal, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT d.id, d.title, d.state, d.value_cents, d.stage_label, d.next_step, d.next_step_due_at, d.next_step_completed_at,
       d.close_window_start, d.close_window_end, d.closed_at, d.closed_outcome, d.last_activity_at, d.created_at, d.updated_at
FROM deals d
JOIN deal_contacts dc ON dc.deal_id = d.id
WHERE d.workspace_id = ? AND dc.contact_id = ?
ORDER BY d.last_activity_at DESC, d.id DESC
LIMIT ?
`, workspaceID, contactID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Deal
	for rows.Next() {
		var d Deal
		if err := rows.Scan(
			&d.ID,
			&d.Title,
			&d.State,
			&d.ValueCents,
			&d.StageLabel,
			&d.NextStep,
			&d.NextStepDueAt,
			&d.NextStepCompleted,
			&d.CloseWindowStart,
			&d.CloseWindowEnd,
			&d.ClosedAt,
			&d.ClosedOutcome,
			&d.LastActivityAt,
			&d.CreatedAt,
			&d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpdateDealNextStep(dealID int64, nextStep string, dueAt *time.Time) (string, error) {
	nextStep = strings.TrimSpace(nextStep)
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return "", err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	var dueAtStr any
	if dueAt != nil {
		dueAtStr = dueAt.UTC().Format(time.RFC3339Nano)
	}

	res, err := s.db.Exec(`
UPDATE deals
SET next_step = ?, next_step_due_at = ?, next_step_completed_at = NULL, updated_at = ?
WHERE workspace_id = ? AND id = ? AND state = 'open'
`, nextStep, dueAtStr, now, workspaceID, dealID)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", sql.ErrNoRows
	}
	return now, nil
}

func (s *Store) CompleteDealNextStep(dealID int64) (string, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`
UPDATE deals
SET next_step_completed_at = ?, updated_at = ?
WHERE workspace_id = ? AND id = ? AND state = 'open' AND next_step != '' AND next_step_completed_at IS NULL
`, now, now, workspaceID, dealID)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", sql.ErrNoRows
	}
	return now, nil
}

func (s *Store) CloseDeal(dealID int64, state, outcome string) (string, error) {
	state = strings.TrimSpace(strings.ToLower(state))
	if state != "won" && state != "lost" {
		return "", errors.New("close state must be won or lost")
	}
	outcome = strings.TrimSpace(outcome)
	if outcome == "" {
		return "", errors.New("close outcome required")
	}

	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`
UPDATE deals
SET state = ?, closed_at = ?, closed_outcome = ?, updated_at = ?
WHERE workspace_id = ? AND id = ? AND state = 'open'
`, state, now, outcome, now, workspaceID, dealID)
	if err != nil {
		return "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", sql.ErrNoRows
	}
	return now, nil
}

func (s *Store) CreateDealEvent(dealID int64, eventType, content string) error {
	return s.CreateDealEventBy(0, dealID, eventType, content)
}

func (s *Store) CreateDealEventBy(actorUserID int64, dealID int64, eventType, content string) error {
	eventType = strings.TrimSpace(strings.ToLower(eventType))
	switch eventType {
	case "note", "call", "email", "meeting", "system":
	default:
		return errors.New("invalid deal event type")
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("deal event content required")
	}

	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var actor any
	if actorUserID > 0 {
		actor = actorUserID
	}
	if _, err := tx.Exec(
		`INSERT INTO deal_events(workspace_id, deal_id, type, content, created_by_user_id, updated_by_user_id, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		workspaceID, dealID, eventType, content, actor, actor, now,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE deals SET last_activity_at = ?, updated_at = ? WHERE workspace_id = ? AND id = ?`,
		now, now, workspaceID, dealID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) ListDealEvents(dealID int64, limit int) ([]DealEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`
SELECT
  e.id, e.deal_id, e.type, e.content,
  e.created_by_user_id, uc.name,
  e.updated_by_user_id, uu.name,
  e.created_at
FROM deal_events e
LEFT JOIN users uc ON uc.id = e.created_by_user_id
LEFT JOIN users uu ON uu.id = e.updated_by_user_id
WHERE e.workspace_id = ? AND e.deal_id = ?
ORDER BY e.created_at DESC, e.id DESC
LIMIT ?
`, workspaceID, dealID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DealEvent
	for rows.Next() {
		var ev DealEvent
		if err := rows.Scan(
			&ev.ID, &ev.DealID, &ev.Type, &ev.Content,
			&ev.CreatedByID, &ev.CreatedBy,
			&ev.UpdatedByID, &ev.UpdatedBy,
			&ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
