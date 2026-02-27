package tenantdb

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

const (
	ActorKindHuman  = "human"
	ActorKindAgent  = "agent"
	ActorKindSystem = "system"
)

const (
	ActivityStatusDone     = "done"
	ActivityStatusCurrent  = "current"
	ActivityStatusError    = "error"
	ActivityStatusCanceled = "canceled"
	ActivityStatusPaused   = "paused"
	ActivityStatusStaged   = "staged"
	ActivityStatusProposed = "proposed"
)

type CreateActivityEventInput struct {
	ActorKind   string
	ActorUserID int64

	Verb       string
	ObjectType string
	ObjectID   *int64

	Status string
	Title  string

	Summary    string
	DetailJSON string
	CreatedAt  *time.Time
}

func (s *Store) CreateActivityEvent(in CreateActivityEventInput) (int64, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return 0, err
	}
	in.ActorKind = strings.TrimSpace(strings.ToLower(in.ActorKind))
	switch in.ActorKind {
	case ActorKindHuman, ActorKindAgent, ActorKindSystem:
	default:
		return 0, errors.New("invalid actor kind")
	}

	in.Status = strings.TrimSpace(strings.ToLower(in.Status))
	if in.Status == "" {
		in.Status = ActivityStatusDone
	}
	switch in.Status {
	case ActivityStatusDone, ActivityStatusCurrent, ActivityStatusError, ActivityStatusCanceled, ActivityStatusPaused, ActivityStatusStaged, ActivityStatusProposed:
	default:
		return 0, errors.New("invalid activity status")
	}

	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return 0, errors.New("title required")
	}
	in.Verb = strings.TrimSpace(in.Verb)
	in.ObjectType = strings.TrimSpace(in.ObjectType)
	in.Summary = strings.TrimSpace(in.Summary)

	createdAt := time.Now().UTC()
	if in.CreatedAt != nil {
		createdAt = in.CreatedAt.UTC()
	}

	var actor any
	if in.ActorUserID > 0 {
		actor = in.ActorUserID
	}
	var objectID any
	if in.ObjectID != nil {
		objectID = *in.ObjectID
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if in.Status == ActivityStatusCurrent {
		if _, err := tx.Exec(`
UPDATE activity_events
SET status = ?
WHERE workspace_id = ? AND actor_kind = ? AND status = ?
`, ActivityStatusDone, workspaceID, in.ActorKind, ActivityStatusCurrent); err != nil {
			return 0, err
		}
	}

	res, err := tx.Exec(`
INSERT INTO activity_events(
  workspace_id, actor_kind, actor_user_id,
  verb, object_type, object_id,
  status, title, summary, detail_json, created_at
) VALUES(?,?,?,?,?,?,?,?,?,?,?)
`, workspaceID, in.ActorKind, actor, in.Verb, in.ObjectType, objectID, in.Status, in.Title, in.Summary, in.DetailJSON, createdAt.Format(time.RFC3339Nano))
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) CurrentActivityEventByActorKind(actorKind string) (*ActivityEvent, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	actorKind = strings.TrimSpace(strings.ToLower(actorKind))
	row := s.db.QueryRow(`
SELECT
  id, actor_kind, actor_user_id,
  verb, object_type, object_id,
  status, title, summary, detail_json,
  created_at
FROM activity_events
WHERE workspace_id = ? AND actor_kind = ? AND status = ?
ORDER BY created_at DESC, id DESC
LIMIT 1
`, workspaceID, actorKind, ActivityStatusCurrent)

	var ev ActivityEvent
	if err := row.Scan(
		&ev.ID, &ev.ActorKind, &ev.ActorUserID,
		&ev.Verb, &ev.ObjectType, &ev.ObjectID,
		&ev.Status, &ev.Title, &ev.Summary, &ev.DetailJSON,
		&ev.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &ev, nil
}

func (s *Store) ListRecentActivityEventsByActorKind(actorKind string, limit int) ([]ActivityEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	actorKind = strings.TrimSpace(strings.ToLower(actorKind))
	rows, err := s.db.Query(`
SELECT
  id, actor_kind, actor_user_id,
  verb, object_type, object_id,
  status, title, summary, detail_json,
  created_at
FROM activity_events
WHERE workspace_id = ? AND actor_kind = ?
ORDER BY created_at DESC, id DESC
LIMIT ?
`, workspaceID, actorKind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ActivityEvent
	for rows.Next() {
		var ev ActivityEvent
		if err := rows.Scan(
			&ev.ID, &ev.ActorKind, &ev.ActorUserID,
			&ev.Verb, &ev.ObjectType, &ev.ObjectID,
			&ev.Status, &ev.Title, &ev.Summary, &ev.DetailJSON,
			&ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *Store) ListRecentNonCurrentActivityEventsByActorKind(actorKind string, limit int) ([]ActivityEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 20
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	actorKind = strings.TrimSpace(strings.ToLower(actorKind))
	rows, err := s.db.Query(`
SELECT
  id, actor_kind, actor_user_id,
  verb, object_type, object_id,
  status, title, summary, detail_json,
  created_at
FROM activity_events
WHERE workspace_id = ? AND actor_kind = ? AND status != ?
ORDER BY created_at DESC, id DESC
LIMIT ?
`, workspaceID, actorKind, ActivityStatusCurrent, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ActivityEvent
	for rows.Next() {
		var ev ActivityEvent
		if err := rows.Scan(
			&ev.ID, &ev.ActorKind, &ev.ActorUserID,
			&ev.Verb, &ev.ObjectType, &ev.ObjectID,
			&ev.Status, &ev.Title, &ev.Summary, &ev.DetailJSON,
			&ev.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

