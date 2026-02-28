package tenantdb

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

type AppendLedgerEventInput struct {
	EventVersion int

	ActorKind   string
	ActorUserID int64

	Op         string
	EntityType string
	EntityID   *int64

	PayloadJSON  string
	Reason       string
	EvidenceJSON string

	CausedByEventID *int64
	ReplacesEventID *int64
	InverseOfEventID *int64

	IdempotencyKey string
	CreatedAt      *time.Time
}

func (s *Store) AppendLedgerEvent(in AppendLedgerEventInput) (int64, error) {
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

	in.Op = strings.TrimSpace(in.Op)
	if in.Op == "" {
		return 0, errors.New("op required")
	}
	in.EntityType = strings.TrimSpace(in.EntityType)
	if in.EntityType == "" {
		return 0, errors.New("entity_type required")
	}

	eventVersion := in.EventVersion
	if eventVersion <= 0 {
		eventVersion = 1
	}

	createdAt := time.Now().UTC()
	if in.CreatedAt != nil {
		createdAt = in.CreatedAt.UTC()
	}

	var actor any
	if in.ActorUserID > 0 {
		actor = in.ActorUserID
	}
	var entityID any
	if in.EntityID != nil {
		entityID = *in.EntityID
	}

	var causedBy any
	if in.CausedByEventID != nil {
		causedBy = *in.CausedByEventID
	}
	var replaces any
	if in.ReplacesEventID != nil {
		replaces = *in.ReplacesEventID
	}
	var inverseOf any
	if in.InverseOfEventID != nil {
		inverseOf = *in.InverseOfEventID
	}

	key := strings.TrimSpace(in.IdempotencyKey)
	var idempotency any
	if key != "" {
		idempotency = key
	}

	return appendLedgerEventExec(
		s.db, workspaceID,
		eventVersion, createdAt.Format(time.RFC3339Nano),
		in.ActorKind, actor,
		in.Op, in.EntityType, entityID,
		in.PayloadJSON, in.Reason, in.EvidenceJSON,
		causedBy, replaces, inverseOf,
		idempotency,
	)
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func appendLedgerEventExec(
	ex execer,
	workspaceID int64,
	eventVersion int,
	createdAt string,
	actorKind string,
	actorUserID any,
	op string,
	entityType string,
	entityID any,
	payloadJSON string,
	reason string,
	evidenceJSON string,
	causedBy any,
	replaces any,
	inverseOf any,
	idempotency any,
) (int64, error) {
	res, err := ex.Exec(`
INSERT INTO ledger_events(
  workspace_id, event_version, created_at,
  actor_kind, actor_user_id,
  op, entity_type, entity_id,
  payload_json, reason, evidence_json,
  caused_by_event_id, replaces_event_id, inverse_of_event_id,
  idempotency_key
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
`, workspaceID, eventVersion, createdAt,
		actorKind, actorUserID,
		op, entityType, entityID,
		payloadJSON, reason, evidenceJSON,
		causedBy, replaces, inverseOf,
		idempotency,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) ListLedgerEvents(limit int) ([]LedgerEvent, error) {
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
SELECT
  id, event_version,
  actor_kind, actor_user_id,
  op, entity_type, entity_id,
  payload_json, reason, evidence_json,
  caused_by_event_id, replaces_event_id, inverse_of_event_id,
  idempotency_key,
  created_at
FROM ledger_events
WHERE workspace_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?
`, workspaceID, limit)
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

func (s *Store) ListLedgerEventsByActorKindAndOp(actorKind, op string, limit int) ([]LedgerEvent, error) {
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	actorKind = strings.TrimSpace(strings.ToLower(actorKind))
	if actorKind == "" {
		return nil, errors.New("actor_kind required")
	}
	op = strings.TrimSpace(op)
	if op == "" {
		return nil, errors.New("op required")
	}

	rows, err := s.db.Query(`
SELECT
  id, event_version,
  actor_kind, actor_user_id,
  op, entity_type, entity_id,
  payload_json, reason, evidence_json,
  caused_by_event_id, replaces_event_id, inverse_of_event_id,
  idempotency_key,
  created_at
FROM ledger_events
WHERE workspace_id = ? AND actor_kind = ? AND op = ?
ORDER BY created_at DESC, id DESC
LIMIT ?
`, workspaceID, actorKind, op, limit)
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

func (s *Store) ListLedgerEventsByEntity(entityType string, entityID int64, limit int) ([]LedgerEvent, error) {
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	entityType = strings.TrimSpace(entityType)
	if entityType == "" {
		return nil, errors.New("entity_type required")
	}

	rows, err := s.db.Query(`
SELECT
  id, event_version,
  actor_kind, actor_user_id,
  op, entity_type, entity_id,
  payload_json, reason, evidence_json,
  caused_by_event_id, replaces_event_id, inverse_of_event_id,
  idempotency_key,
  created_at
FROM ledger_events
WHERE workspace_id = ? AND entity_type = ? AND entity_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?
`, workspaceID, entityType, entityID, limit)
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

type LedgerEventFilter struct {
	ActorKind  string
	Op         string
	EntityType string
	EntityID   *int64
	Limit      int
}

func (s *Store) ListLedgerEventsFiltered(f LedgerEventFilter) ([]LedgerEvent, error) {
	limit := f.Limit
	if limit <= 0 || limit > 5000 {
		limit = 200
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}

	var where []string
	args := []any{workspaceID}

	where = append(where, "workspace_id = ?")
	if actor := strings.TrimSpace(strings.ToLower(f.ActorKind)); actor != "" {
		where = append(where, "actor_kind = ?")
		args = append(args, actor)
	}
	if op := strings.TrimSpace(f.Op); op != "" {
		where = append(where, "op = ?")
		args = append(args, op)
	}
	if et := strings.TrimSpace(f.EntityType); et != "" {
		where = append(where, "entity_type = ?")
		args = append(args, et)
	}
	if f.EntityID != nil {
		where = append(where, "entity_id = ?")
		args = append(args, *f.EntityID)
	}
	args = append(args, limit)

	rows, err := s.db.Query(`
SELECT
  id, event_version,
  actor_kind, actor_user_id,
  op, entity_type, entity_id,
  payload_json, reason, evidence_json,
  caused_by_event_id, replaces_event_id, inverse_of_event_id,
  idempotency_key,
  created_at
FROM ledger_events
WHERE `+strings.Join(where, " AND ")+`
ORDER BY created_at DESC, id DESC
LIMIT ?
`, args...)
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

func (s *Store) LedgerEventByID(eventID int64) (*LedgerEvent, error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(`
SELECT
  id, event_version,
  actor_kind, actor_user_id,
  op, entity_type, entity_id,
  payload_json, reason, evidence_json,
  caused_by_event_id, replaces_event_id, inverse_of_event_id,
  idempotency_key,
  created_at
FROM ledger_events
WHERE workspace_id = ? AND id = ?
`, workspaceID, eventID)

	var ev LedgerEvent
	if err := row.Scan(
		&ev.ID, &ev.EventVersion,
		&ev.ActorKind, &ev.ActorUserID,
		&ev.Op, &ev.EntityType, &ev.EntityID,
		&ev.PayloadJSON, &ev.Reason, &ev.EvidenceJSON,
		&ev.CausedByEventID, &ev.ReplacesEventID, &ev.InverseOfEventID,
		&ev.IdempotencyKey,
		&ev.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &ev, nil
}
