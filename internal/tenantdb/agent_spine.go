package tenantdb

import (
	"encoding/json"
	"strings"
)

type AgentSpineEventInput struct {
	Status     string
	Title      string
	Summary    string
	DetailJSON string
}

// AppendAgentSpineEvent appends an agent-authored spine event to the ledger.
func (s *Store) AppendAgentSpineEvent(actorUserID int64, in AgentSpineEventInput) (int64, error) {
	status := strings.TrimSpace(strings.ToLower(in.Status))
	if status == "" {
		status = ActivityStatusDone
	}
	switch status {
	case ActivityStatusDone, ActivityStatusCurrent, ActivityStatusError, ActivityStatusCanceled, ActivityStatusPaused, ActivityStatusStaged, ActivityStatusProposed:
	default:
		status = ActivityStatusDone
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "Agent"
	}

	payload, _ := json.Marshal(map[string]any{
		"status":      status,
		"title":       title,
		"summary":     strings.TrimSpace(in.Summary),
		"detail_json": strings.TrimSpace(in.DetailJSON),
	})
	return s.AppendLedgerEvent(AppendLedgerEventInput{
		ActorKind:   ActorKindAgent,
		ActorUserID: actorUserID,
		Op:          "agent.spine.event",
		EntityType:  "agent_spine",
		PayloadJSON: string(payload),
	})
}

