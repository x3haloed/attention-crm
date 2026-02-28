package app

import (
	"attention-crm/internal/tenantdb"
	"html/template"
	"time"
)

func loadAgentRail(db *tenantdb.Store) (template.HTML, error) {
	ledger, err := db.ListLedgerEventsByActorKindAndOp(tenantdb.ActorKindAgent, "agent.spine.event", 25)
	if err != nil {
		return "", err
	}
	past, current := splitAgentSpineEvents(ledger)
	return renderAgentRail(time.Now(), past, current), nil
}

