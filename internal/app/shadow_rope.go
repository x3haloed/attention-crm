package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type shadowRopeItem struct {
	LedgerEventID int64  `json:"ledger_event_id"`
	CreatedAt     string `json:"created_at"`
	ActorKind     string `json:"actor_kind"`
	ActorUserID   *int64 `json:"actor_user_id,omitempty"`
	EntityType    string `json:"entity_type"`
	EntityID      *int64 `json:"entity_id,omitempty"`
	Op            string `json:"-"`

	Narration string `json:"narration"`
	Detail    string `json:"detail,omitempty"`
}

type shadowRopeMarker struct {
	BeforeLedgerEventID int64  `json:"before_ledger_event_id"`
	BeforeCreatedAt     string `json:"before_created_at"`
}

type shadowRopeState struct {
	LastSeenLedgerEventID int64
	Marker                *shadowRopeMarker
	Items                 []shadowRopeItem
}

func shadowSessionKey(r *http.Request, sess session, tenant control.Tenant) string {
	base := tenant.Slug + "|" + strconv.FormatInt(sess.UserID, 10)
	if r != nil {
		if c, err := r.Cookie("attention_session"); err == nil && strings.TrimSpace(c.Value) != "" {
			base = base + "|" + strings.TrimSpace(c.Value)
		} else {
			base = base + "|" + strings.TrimSpace(r.RemoteAddr) + "|" + strings.TrimSpace(r.UserAgent())
		}
	}
	sum := sha256.Sum256([]byte(base))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Server) shadowRopeSnapshot(db *tenantdb.Store, tenant control.Tenant, sess session, r *http.Request) (shadowRopeMarker, []shadowRopeItem, int, error) {
	key := shadowSessionKey(r, sess, tenant)

	s.ropeMu.Lock()
	state := s.rope[key]
	if state == nil {
		state = &shadowRopeState{}
		s.rope[key] = state
	}
	lastSeen := state.LastSeenLedgerEventID
	s.ropeMu.Unlock()

	// Pull a recent window and add any events beyond the cursor. (Cheap v0; can optimize later.)
	recent, err := db.ListLedgerEventsFiltered(tenantdb.LedgerEventFilter{Limit: 250})
	if err != nil {
		return shadowRopeMarker{}, nil, 0, err
	}

	var newEvents []tenantdb.LedgerEvent
	for _, ev := range recent {
		if ev.ID > lastSeen {
			newEvents = append(newEvents, ev)
		}
	}
	sort.Slice(newEvents, func(i, j int) bool { return newEvents[i].ID < newEvents[j].ID })

	items := make([]shadowRopeItem, 0, len(newEvents))
	triggerAdded := 0
	for _, ev := range newEvents {
		// Pairing: include the current human's events; always include agent/system.
		if strings.EqualFold(ev.ActorKind, tenantdb.ActorKindHuman) {
			if ev.ActorUserID.Valid && ev.ActorUserID.Int64 != sess.UserID {
				continue
			}
		}
		it, ok := narrateLedgerEvent(db, ev)
		if !ok {
			continue
		}
		items = append(items, it)
		lastSeen = ev.ID

		// Avoid self-trigger loops: agent spine events produced by ui.message should not retrigger shadow mode.
		if !(strings.EqualFold(ev.ActorKind, tenantdb.ActorKindAgent) && strings.EqualFold(strings.TrimSpace(ev.Op), "agent.spine.event")) {
			triggerAdded++
		}
	}

	s.ropeMu.Lock()
	defer s.ropeMu.Unlock()
	state = s.rope[key]
	if state == nil {
		state = &shadowRopeState{}
		s.rope[key] = state
	}
	state.LastSeenLedgerEventID = lastSeen

	// Append, then trim to a bounded tail.
	state.Items = append(state.Items, items...)
	const maxItems = 80
	if len(state.Items) > maxItems {
		dropped := state.Items[:len(state.Items)-maxItems]
		keep := state.Items[len(state.Items)-maxItems:]
		lastDropped := dropped[len(dropped)-1]
		state.Marker = &shadowRopeMarker{
			BeforeLedgerEventID: lastDropped.LedgerEventID,
			BeforeCreatedAt:     lastDropped.CreatedAt,
		}
		state.Items = keep
	}

	var marker shadowRopeMarker
	if state.Marker != nil {
		marker = *state.Marker
	}
	outItems := append([]shadowRopeItem(nil), state.Items...)
	return marker, outItems, triggerAdded, nil
}

type ropeContactCreatedPayload struct {
	Name    string `json:"name"`
	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`
	Company string `json:"company,omitempty"`
}

type ropeContactFieldSetPayload struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type ropeInteractionCreatedPayload struct {
	ContactID int64  `json:"contact_id"`
	Type      string `json:"type"`
	Content   string `json:"content"`
	DueAt     string `json:"due_at,omitempty"`
}

type ropeEmailCommittedPayload struct {
	ExternalEffectID string   `json:"external_effect_id"`
	To               string   `json:"to"`
	Subject          string   `json:"subject"`
	Body             []string `json:"body"`
}

func narrateLedgerEvent(db *tenantdb.Store, ev tenantdb.LedgerEvent) (shadowRopeItem, bool) {
	op := strings.TrimSpace(ev.Op)
	entityType := strings.TrimSpace(ev.EntityType)

	var actorUserID *int64
	if ev.ActorUserID.Valid && ev.ActorUserID.Int64 > 0 {
		id := ev.ActorUserID.Int64
		actorUserID = &id
	}
	var entityID *int64
	if ev.EntityID.Valid {
		id := ev.EntityID.Int64
		entityID = &id
	}

	actorPrefix := "System"
	if strings.EqualFold(ev.ActorKind, tenantdb.ActorKindHuman) {
		actorPrefix = "You"
	} else if strings.EqualFold(ev.ActorKind, tenantdb.ActorKindAgent) {
		actorPrefix = "Agent"
	}

	narration := ""
	detail := ""
	switch op {
	case "contact.created":
		var p ropeContactCreatedPayload
		_ = json.Unmarshal([]byte(strings.TrimSpace(ev.PayloadJSON)), &p)
		name := strings.TrimSpace(p.Name)
		if name == "" {
			name = "a contact"
		}
		narration = actorPrefix + " created " + name
		if company := strings.TrimSpace(p.Company); company != "" {
			narration = narration + " (" + company + ")"
		}

	case "contact.field.set":
		var p ropeContactFieldSetPayload
		_ = json.Unmarshal([]byte(strings.TrimSpace(ev.PayloadJSON)), &p)
		field := strings.TrimSpace(p.Field)
		if field == "" {
			field = "a field"
		}
		contactName := "a contact"
		if db != nil && ev.EntityID.Valid && ev.EntityID.Int64 > 0 {
			if c, err := db.ContactByID(ev.EntityID.Int64); err == nil {
				if name := strings.TrimSpace(c.Name); name != "" {
					contactName = name
				}
			}
		}
		narration = actorPrefix + " updated " + field + " on " + contactName
		if v := strings.TrimSpace(p.Value); v != "" {
			detail = field + ": " + v
		}

	case "interaction.created":
		var p ropeInteractionCreatedPayload
		_ = json.Unmarshal([]byte(strings.TrimSpace(ev.PayloadJSON)), &p)
		kind := strings.TrimSpace(strings.ToLower(p.Type))
		if kind == "" {
			kind = "interaction"
		}
		contactName := "a contact"
		if db != nil && p.ContactID > 0 {
			if c, err := db.ContactByID(p.ContactID); err == nil {
				if name := strings.TrimSpace(c.Name); name != "" {
					contactName = name
				}
			}
		}
		content := strings.TrimSpace(p.Content)
		excerpt := content
		const maxExcerpt = 96
		if len([]rune(excerpt)) > maxExcerpt {
			excerpt = string([]rune(excerpt)[:maxExcerpt]) + "…"
		}
		if excerpt != "" {
			narration = actorPrefix + " logged a " + kind + " with " + contactName + ": “" + excerpt + "”"
		} else {
			narration = actorPrefix + " logged a " + kind + " with " + contactName
		}
		if content != "" {
			detail = content
		}
		if due := strings.TrimSpace(p.DueAt); due != "" {
			if detail != "" {
				detail = detail + "\n"
			}
			detail = detail + "Follow-up due: " + due
		}

	case "interaction.completed":
		narration = actorPrefix + " completed a follow-up"

	case "email.send.committed":
		var p ropeEmailCommittedPayload
		_ = json.Unmarshal([]byte(strings.TrimSpace(ev.PayloadJSON)), &p)
		if strings.TrimSpace(p.To) != "" {
			narration = actorPrefix + " sent an email to " + strings.TrimSpace(p.To)
		} else {
			narration = actorPrefix + " sent an email"
		}
		if subj := strings.TrimSpace(p.Subject); subj != "" {
			detail = "Subject: " + subj
		}
		if len(p.Body) > 0 {
			if detail != "" {
				detail = detail + "\n"
			}
			detail = detail + strings.Join(p.Body, "\n")
		}

	default:
		// Do not leak internal op names into the rope narration.
		if entityType != "" {
			narration = actorPrefix + " recorded a change to " + entityType
		} else {
			narration = actorPrefix + " recorded a change"
		}
	}

	// Ensure timestamps are present and normalized for UI display.
	createdAt := strings.TrimSpace(ev.CreatedAt)
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	return shadowRopeItem{
		LedgerEventID: ev.ID,
		CreatedAt:     createdAt,
		ActorKind:     ev.ActorKind,
		ActorUserID:   actorUserID,
		EntityType:    entityType,
		EntityID:      entityID,
		Op:            op,
		Narration:     narration,
		Detail:        detail,
	}, true
}
