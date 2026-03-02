package tenantdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

type BackfillLedgerResult struct {
	ContactsCreated        int `json:"contacts_created"`
	InteractionsCreated    int `json:"interactions_created"`
	InteractionsCompleted  int `json:"interactions_completed"`
	SkippedExisting        int `json:"skipped_existing"`
}

// BackfillLedgerFromCurrentTables emits ledger_events derived from the current contacts/interactions tables.
// This is intended for bootstrapping old demo data into the ledger so shadow ropes have history.
//
// Idempotent: uses unique idempotency keys, so it can be re-run safely.
func BackfillLedgerFromCurrentTables(s *Store, actorUserID int64) (BackfillLedgerResult, error) {
	if s == nil {
		return BackfillLedgerResult{}, errors.New("store required")
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return BackfillLedgerResult{}, err
	}

	var res BackfillLedgerResult
	var maxContactID int64
	var maxInteractionID int64

	type contactRow struct {
		ID        int64
		Name      string
		Email     string
		Phone     string
		Company   string
		Notes     string
		CreatedAt string
	}
	contacts := []contactRow{}
	{
		rows, err := s.db.Query(`
SELECT id, name, email, phone, company, notes, created_at
FROM contacts
WHERE workspace_id = ?
ORDER BY created_at ASC, id ASC
`, workspaceID)
		if err != nil {
			return BackfillLedgerResult{}, err
		}
		defer rows.Close()
		for rows.Next() {
			var c contactRow
			if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Phone, &c.Company, &c.Notes, &c.CreatedAt); err != nil {
				return BackfillLedgerResult{}, err
			}
			contacts = append(contacts, c)
		}
		if err := rows.Err(); err != nil {
			return BackfillLedgerResult{}, err
		}
	}

	for _, c := range contacts {
		if c.ID > maxContactID {
			maxContactID = c.ID
		}
		createdAt := parseMaybeRFC3339(c.CreatedAt)
		payload, _ := json.Marshal(map[string]any{
			"name":    strings.TrimSpace(c.Name),
			"email":   strings.TrimSpace(strings.ToLower(c.Email)),
			"phone":   strings.TrimSpace(c.Phone),
			"company": strings.TrimSpace(c.Company),
			"notes":   strings.TrimSpace(c.Notes),
		})
		id := c.ID
		_, err := s.AppendLedgerEvent(AppendLedgerEventInput{
			ActorKind:      ActorKindHuman,
			ActorUserID:    actorUserID,
			Op:            "contact.created",
			EntityType:     "contact",
			EntityID:       &id,
			PayloadJSON:    string(payload),
			IdempotencyKey: "backfill:v1:contact.created:" + strconv.FormatInt(c.ID, 10),
			CreatedAt:      &createdAt,
		})
		if err != nil {
			if isUniqueConstraintErr(err) {
				res.SkippedExisting++
				continue
			}
			return BackfillLedgerResult{}, err
		}
		res.ContactsCreated++
	}

	type interactionRow struct {
		ID              int64
		ContactID       int64
		Type            string
		Content         string
		DueAt           sql.NullString
		CompletedAt     sql.NullString
		CreatedByUserID sql.NullInt64
		UpdatedByUserID sql.NullInt64
		CreatedAt       string
	}
	interactions := []interactionRow{}
	{
		rows, err := s.db.Query(`
SELECT id, contact_id, type, content, due_at, completed_at, created_by_user_id, updated_by_user_id, created_at
FROM interactions
WHERE workspace_id = ?
ORDER BY created_at ASC, id ASC
`, workspaceID)
		if err != nil {
			return BackfillLedgerResult{}, err
		}
		defer rows.Close()
		for rows.Next() {
			var it interactionRow
			if err := rows.Scan(&it.ID, &it.ContactID, &it.Type, &it.Content, &it.DueAt, &it.CompletedAt, &it.CreatedByUserID, &it.UpdatedByUserID, &it.CreatedAt); err != nil {
				return BackfillLedgerResult{}, err
			}
			interactions = append(interactions, it)
		}
		if err := rows.Err(); err != nil {
			return BackfillLedgerResult{}, err
		}
	}

	for _, it := range interactions {
		if it.ID > maxInteractionID {
			maxInteractionID = it.ID
		}
		createdAt := parseMaybeRFC3339(it.CreatedAt)
		due := ""
		if it.DueAt.Valid {
			due = strings.TrimSpace(it.DueAt.String)
		}
		payload, _ := json.Marshal(map[string]any{
			"contact_id": it.ContactID,
			"type":       strings.TrimSpace(strings.ToLower(it.Type)),
			"content":    strings.TrimSpace(it.Content),
			"due_at":     due,
		})
		id := it.ID
		actor := actorUserID
		if it.CreatedByUserID.Valid && it.CreatedByUserID.Int64 > 0 {
			actor = it.CreatedByUserID.Int64
		}
		_, err := s.AppendLedgerEvent(AppendLedgerEventInput{
			ActorKind:      ActorKindHuman,
			ActorUserID:    actor,
			Op:            "interaction.created",
			EntityType:     "interaction",
			EntityID:       &id,
			PayloadJSON:    string(payload),
			IdempotencyKey: "backfill:v1:interaction.created:" + strconv.FormatInt(it.ID, 10),
			CreatedAt:      &createdAt,
		})
		if err != nil {
			if isUniqueConstraintErr(err) {
				res.SkippedExisting++
			} else {
				return BackfillLedgerResult{}, err
			}
		} else {
			res.InteractionsCreated++
		}

		if it.CompletedAt.Valid && strings.TrimSpace(it.CompletedAt.String) != "" {
			completedAt := parseMaybeRFC3339(it.CompletedAt.String)
			actor2 := actorUserID
			if it.UpdatedByUserID.Valid && it.UpdatedByUserID.Int64 > 0 {
				actor2 = it.UpdatedByUserID.Int64
			}
			_, err := s.AppendLedgerEvent(AppendLedgerEventInput{
				ActorKind:      ActorKindHuman,
				ActorUserID:    actor2,
				Op:            "interaction.completed",
				EntityType:     "interaction",
				EntityID:       &id,
				PayloadJSON:    "{}",
				IdempotencyKey: "backfill:v1:interaction.completed:" + strconv.FormatInt(it.ID, 10),
				CreatedAt:      &completedAt,
			})
			if err != nil {
				if isUniqueConstraintErr(err) {
					res.SkippedExisting++
				} else {
					return BackfillLedgerResult{}, err
				}
			} else {
				res.InteractionsCompleted++
			}
		}
	}

	if err := bumpEntityCounterToAtLeast(s.db, workspaceID, "contact", maxContactID+1); err != nil {
		return BackfillLedgerResult{}, err
	}
	if err := bumpEntityCounterToAtLeast(s.db, workspaceID, "interaction", maxInteractionID+1); err != nil {
		return BackfillLedgerResult{}, err
	}

	return res, nil
}

func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "constraint failed")
}

func parseMaybeRFC3339(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now().UTC()
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC()
	}
	return time.Now().UTC()
}

func bumpEntityCounterToAtLeast(db *sql.DB, workspaceID int64, entityType string, nextID int64) error {
	if db == nil {
		return errors.New("db required")
	}
	if workspaceID <= 0 {
		return errors.New("workspace_id required")
	}
	entityType = strings.TrimSpace(entityType)
	if entityType == "" {
		return errors.New("entity_type required")
	}
	if nextID <= 1 {
		// Nothing to do; callers pass max+1.
		return nil
	}

	_, err := db.Exec(`
INSERT INTO entity_id_counters(workspace_id, entity_type, next_id)
VALUES(?,?,?)
ON CONFLICT(workspace_id, entity_type) DO UPDATE SET
  next_id = CASE
    WHEN entity_id_counters.next_id <= excluded.next_id THEN excluded.next_id
    ELSE entity_id_counters.next_id
  END
`, workspaceID, entityType, nextID)
	return err
}
