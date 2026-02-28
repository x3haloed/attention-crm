package tenantdb

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func randHex(nbytes int) (string, error) {
	if nbytes <= 0 || nbytes > 64 {
		nbytes = 16
	}
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateEmailSendCommitted appends an irreversible boundary event and applies projections (outbox + optional read models).
// This does not actually send email; delivery is handled by the outbox dispatcher.
func (s *Store) CreateEmailSendCommitted(actorUserID int64, to, subject string, body []string) (commitEventID int64, emailEntityID int64, externalEffectID string, err error) {
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return 0, 0, "", err
	}
	to = strings.TrimSpace(to)
	subject = strings.TrimSpace(subject)
	if to == "" {
		return 0, 0, "", errors.New("to required")
	}
	if subject == "" {
		return 0, 0, "", errors.New("subject required")
	}
	if len(body) == 0 {
		return 0, 0, "", errors.New("body required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, "", err
	}
	defer tx.Rollback()

	emailID, err := allocateEntityIDTx(tx, workspaceID, "email")
	if err != nil {
		return 0, 0, "", err
	}
	effectID, err := randHex(16)
	if err != nil {
		return 0, 0, "", err
	}

	payload, _ := json.Marshal(emailCommittedPayload{
		ExternalEffectID: effectID,
		To:               to,
		Subject:          subject,
		Body:             body,
	})

	var actor any
	if actorUserID > 0 {
		actor = actorUserID
	}
	createdAt := time.Now().UTC().Format(time.RFC3339Nano)
	evID, err := appendLedgerEventExec(
		tx,
		workspaceID,
		1,
		createdAt,
		ActorKindHuman,
		actor,
		"email.send.committed",
		"email",
		emailID,
		string(payload),
		"",
		"",
		nil, nil, nil,
		nil,
	)
	if err != nil {
		return 0, 0, "", err
	}

	// Apply outbox projection in-transaction so the effect is immediately enqueue'd.
	if err := (OutboxProjection{}).Apply(tx, workspaceID, LedgerEvent{
		ID:         evID,
		EventVersion: 1,
		ActorKind:  ActorKindHuman,
		ActorUserID: sql.NullInt64{Int64: actorUserID, Valid: actorUserID > 0},
		Op:         "email.send.committed",
		EntityType: "email",
		EntityID:   sql.NullInt64{Int64: emailID, Valid: true},
		PayloadJSON: string(payload),
		CreatedAt:  createdAt,
	}); err != nil {
		return 0, 0, "", err
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, "", err
	}
	// Advance projection cursor (idempotent due to UNIQUE constraint on commit_event_id).
	_ = s.ApplyProjection(OutboxProjection{})
	return evID, emailID, effectID, nil
}

type OutboxProcessResult struct {
	Processed int
	Sent      int
	Failed    int
}

// ProcessOutboxOnce processes up to `limit` pending outbox rows and records delivery observation events.
// This is a stub dispatcher: it does not call an email provider yet.
func (s *Store) ProcessOutboxOnce(limit int) (OutboxProcessResult, error) {
	if limit <= 0 || limit > 500 {
		limit = 25
	}
	workspaceID, err := s.primaryWorkspaceID()
	if err != nil {
		return OutboxProcessResult{}, err
	}

	var res OutboxProcessResult
	for i := 0; i < limit; i++ {
		eff, err := s.claimNextPendingOutbox(workspaceID)
		if err != nil {
			return res, err
		}
		if eff == nil {
			return res, nil
		}
		res.Processed++

		// Stub "send": always succeed.
		providerID := "mock:" + eff.ExternalEffectID
		if err := s.markOutboxSentAndObserve(workspaceID, *eff, providerID); err != nil {
			res.Failed++
			continue
		}
		res.Sent++
	}
	return res, nil
}

func (s *Store) claimNextPendingOutbox(workspaceID int64) (*OutboxEffect, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRow(`
SELECT
  id, kind, status,
  email_entity_id, commit_event_id, external_effect_id,
  payload_json,
  attempt_count, next_attempt_at, last_error,
  created_at, updated_at, sent_at
FROM outbox_effects
WHERE workspace_id = ?
  AND status = 'pending'
  AND (next_attempt_at IS NULL OR next_attempt_at <= (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')))
ORDER BY id ASC
LIMIT 1
`, workspaceID)

	var eff OutboxEffect
	if err := row.Scan(
		&eff.ID, &eff.Kind, &eff.Status,
		&eff.EmailEntityID, &eff.CommitEventID, &eff.ExternalEffectID,
		&eff.PayloadJSON,
		&eff.AttemptCount, &eff.NextAttemptAt, &eff.LastError,
		&eff.CreatedAt, &eff.UpdatedAt, &eff.SentAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if _, err := tx.Exec(`
UPDATE outbox_effects
SET status = 'processing',
    attempt_count = attempt_count + 1,
    updated_at = (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
WHERE workspace_id = ? AND id = ? AND status = 'pending'
`, workspaceID, eff.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &eff, nil
}

func (s *Store) markOutboxSentAndObserve(workspaceID int64, eff OutboxEffect, providerMessageID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.Exec(`
UPDATE outbox_effects
SET status = 'sent',
    sent_at = ?,
    updated_at = ?,
    last_error = ''
WHERE workspace_id = ? AND id = ?
`, now, now, workspaceID, eff.ID); err != nil {
		return err
	}

	obsPayload, _ := json.Marshal(map[string]any{
		"external_effect_id": eff.ExternalEffectID,
		"provider_message_id": strings.TrimSpace(providerMessageID),
	})

	var emailEntity any
	if eff.EmailEntityID.Valid && eff.EmailEntityID.Int64 > 0 {
		emailEntity = eff.EmailEntityID.Int64
	}
	_, err = appendLedgerEventExec(
		tx,
		workspaceID,
		1,
		now,
		ActorKindSystem,
		nil,
		"email.send.delivered",
		"email",
		emailEntity,
		string(obsPayload),
		"",
		"",
		eff.CommitEventID, nil, nil,
		nil,
	)
	if err != nil {
		return fmt.Errorf("append delivered event: %w", err)
	}

	return tx.Commit()
}
