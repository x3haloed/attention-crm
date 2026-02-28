package tenantdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// ActivityEventsProjection is a v0 projection that turns ledger events into the existing
// activity_events table used by the current UI.
//
// This is intentionally mutable: projections are rebuildable read models.
type ActivityEventsProjection struct{}

func (p ActivityEventsProjection) Name() string { return "activity_events" }

func (p ActivityEventsProjection) Reset(tx *sql.Tx, workspaceID int64) error {
	_, err := tx.Exec(`DELETE FROM activity_events WHERE workspace_id = ?`, workspaceID)
	return err
}

type activityPayload struct {
	Status    string `json:"status"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	DetailJSON string `json:"detail_json"`
	Verb      string `json:"verb"`
	ObjectType string `json:"object_type"`
	ObjectID  *int64 `json:"object_id"`
}

func (p ActivityEventsProjection) Apply(tx *sql.Tx, workspaceID int64, ev LedgerEvent) error {
	// Only project explicit spine events for now.
	if ev.Op != "agent.spine.event" {
		return nil
	}
	if ev.ActorKind != ActorKindAgent {
		return nil
	}
	raw := strings.TrimSpace(ev.PayloadJSON)
	if raw == "" {
		return errors.New("missing payload_json")
	}
	var payload activityPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return err
	}

	status := strings.TrimSpace(strings.ToLower(payload.Status))
	if status == "" {
		status = ActivityStatusDone
	}
	switch status {
	case ActivityStatusDone, ActivityStatusCurrent, ActivityStatusError, ActivityStatusCanceled, ActivityStatusPaused, ActivityStatusStaged, ActivityStatusProposed:
	default:
		return errors.New("invalid activity status")
	}
	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = strings.TrimSpace(ev.Reason)
	}
	if title == "" {
		title = "Activity"
	}
	summary := strings.TrimSpace(payload.Summary)
	detail := strings.TrimSpace(payload.DetailJSON)
	verb := strings.TrimSpace(payload.Verb)
	objectType := strings.TrimSpace(payload.ObjectType)
	var objectID any
	if payload.ObjectID != nil {
		objectID = *payload.ObjectID
	}

	// Maintain "current" by mutating the projection only.
	if status == ActivityStatusCurrent {
		if _, err := tx.Exec(`
UPDATE activity_events
SET status = ?
WHERE workspace_id = ? AND actor_kind = ? AND status = ?
`, ActivityStatusDone, workspaceID, ActorKindAgent, ActivityStatusCurrent); err != nil {
			return err
		}
	}

	var actor any
	if ev.ActorUserID.Valid && ev.ActorUserID.Int64 > 0 {
		actor = ev.ActorUserID.Int64
	}
	_, err := tx.Exec(`
INSERT INTO activity_events(
  workspace_id, actor_kind, actor_user_id,
  verb, object_type, object_id,
  status, title, summary, detail_json, created_at
) VALUES(?,?,?,?,?,?,?,?,?,?,?)
`, workspaceID, ActorKindAgent, actor, verb, objectType, objectID, status, title, summary, detail, ev.CreatedAt)
	return err
}

