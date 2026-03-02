package app

import (
	"attention-crm/internal/tenantdb"
	"encoding/json"
	"html/template"
	"strings"
	"time"
)

func loadAgentRail(db *tenantdb.Store, marker *shadowRopeMarker) (template.HTML, error) {
	ledger, err := db.ListLedgerEventsFiltered(tenantdb.LedgerEventFilter{
		Limit: 60,
	})
	if err != nil {
		return "", err
	}

	past, current := buildRailSpineEvents(ledger, 10)
	return renderAgentRail(time.Now(), marker, past, current), nil
}

func buildRailSpineEvents(events []tenantdb.LedgerEvent, maxPast int) ([]spineEvent, *spineEvent) {
	if maxPast <= 0 || maxPast > 50 {
		maxPast = 10
	}

	var current *spineEvent

	// First pass: identify current.
	for _, ev := range events {
		if ev.ActorKind == tenantdb.ActorKindAgent && strings.EqualFold(strings.TrimSpace(ev.Op), "agent.spine.event") {
			raw := strings.TrimSpace(ev.PayloadJSON)
			if raw == "" {
				continue
			}
			var payload spinePayload
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				continue
			}
			title := strings.TrimSpace(payload.Title)
			if title == "" {
				continue
			}
			if current == nil && strings.EqualFold(strings.TrimSpace(payload.Status), "current") {
				out := spineEvent{
					ActorKind:  ev.ActorKind,
					Title:      title,
					Summary:    strings.TrimSpace(payload.Summary),
					DetailJSON: strings.TrimSpace(payload.DetailJSON),
					CreatedAt:  ev.CreatedAt,
				}
				tmp := out
				current = &tmp
				continue
			}
			continue
		}
	}

	var past []spineEvent

	// Second pass: select most-recent agent events (excluding current).
	for _, ev := range events {
		if ev.ActorKind == tenantdb.ActorKindAgent && strings.EqualFold(strings.TrimSpace(ev.Op), "agent.spine.event") {
			raw := strings.TrimSpace(ev.PayloadJSON)
			if raw == "" {
				continue
			}
			var payload spinePayload
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				continue
			}
			title := strings.TrimSpace(payload.Title)
			if title == "" {
				continue
			}
			if current != nil && strings.EqualFold(strings.TrimSpace(payload.Status), "current") {
				continue
			}
			past = append(past, spineEvent{
				ActorKind:  ev.ActorKind,
				Title:      title,
				Summary:    strings.TrimSpace(payload.Summary),
				DetailJSON: strings.TrimSpace(payload.DetailJSON),
				CreatedAt:  ev.CreatedAt,
			})
		}

		if len(past) >= maxPast {
			break
		}
	}

	return past, current
}
