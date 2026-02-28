# Mutual Legibility Ledger (Append-Only)

This project adopts an **append-only mutual ledger** as a core trust primitive.

The intent is not “logging” for debugging. The ledger is the system’s *memory of actions*:

- **Legibility:** what happened, by whom, and why (in human terms).
- **Reversibility:** undo/redo is expressed as additional events, not destructive mutation.
- **Co-agency:** human + agent share the same action surface and vocabulary.

## Source of Truth

The ledger is the **source of truth** for meaningful state changes.

“Current tables” (contacts, interactions, deals, etc.) are treated as **projections** of the ledger:

- Fast read models (indexes / materialized views / cached tables).
- Rebuildable from the event stream.
- Allowed to change shape without losing history.

## Event Shape (Conceptual)

Events must be:

- **Append-only**
- **Attributable** (`actor_kind`, optional `actor_user_id`)
- **Idempotent** (via `idempotency_key` where applicable)
- **Causally linkable** (chains via `caused_by_event_id`)
- **Versioned** (`event_version`) to support evolution

Minimum conceptual fields:

- `event_id`, `ts`
- `actor_kind` (`human|agent|system`), optional `actor_user_id`
- `op` (verb), `entity_type`, `entity_id`
- `payload` (JSON), `reason` (human-readable), `evidence` (optional pointers)
- `caused_by_event_id` (optional)

## Undo / Redo

Undo/redo is not “rewinding tables”. It is **new events that supersede prior effects**.

- **Undo**: append an event that references a prior event and compensates/supersedes it
  - Example: `contact.name.set` undone by another `contact.name.set` with the previous value.
- **Redo**: append a new forward event (often re-applying the original op with a new id)

This avoids destructive mutation while preserving a coherent history.

## External Effects (Email, Calendar, etc.)

External effects are special: once the outside world changes, there is no true rollback.

We still keep the ledger model consistent by making “send an email” a **two-phase, witness-paced sequence**.

### 1) How external effects affect how an event is stored

Model the effect as a **state machine** recorded in the ledger (not one monolithic “sent” event):

1. `email.draft.created` (reversible)
2. `email.preview.ready` (reversible)
3. `email.send.committed` (irreversible boundary)
4. `email.send.delivered` / `email.send.failed` (observations)

Important properties:

- The **committed** event carries an immutable payload snapshot (recipients, subject/body, headers),
  plus a stable `external_effect_id` for correlation with delivery provider callbacks.
- “Draft” and “Preview” are internal state and must be undoable by superseding events.

### 2) How it presents so it does not prevent “roll back before external email happened”

We distinguish two different “rollbacks”:

- **Internal state rollback (projection time-travel):** the UI can render the desk as-of any event id,
  even if later irreversible effects occurred.
- **World rollback:** not possible for external effects; instead we represent compensation.

In the event stream UI:

- The `email.send.committed` event is a **hard boundary** (clearly labeled “sent”).
- “Time travel” views can stop before that boundary (so the user can inspect the world as it was),
  but the stream still shows that the commit happened later.
- Compensation appears as a new event, e.g. `email.followup.sent` / `email.apology.sent` / `email.thread.closed`,
  rather than pretending the original send never occurred.

This preserves user trust: the system never lies about the outside world.

### 3) Delegating trust for non-reversible effects (future schema)

For agents to perform irreversible actions, we will need an explicit authorization model recorded in the ledger:

- **Autonomy level** (observe → suggest → stage → commit)
- **Effect risk class** (low/medium/high)
- **Required witness gate** (always/threshold-based/never)
- **Policy + capability grants** (who allowed what; scope; expiration)
- **Intervention window** metadata (how long; how it was surfaced; user decision)

This will let us answer, for any external effect:

- Who authorized it?
- Under what policy?
- What was the intervention window?
- What was committed, exactly?
- What compensations occurred afterward?

#### Long-running approvals (important)

We explicitly accept that “approval” is not always a one-click, one-event gate.

Long term, approvals must be able to be:

- **Long-running** (minutes → days), with explicit expiration.
- **Scoped** (to a class of actions / a specific relationship / a campaign), not just a single event.
- **Multi-event** (a grant that covers multiple subsequent commits) with revocation and audit.

Autonomy requires this, because an agent cannot be useful if every low-risk action requires a new approval event.

## Why this is the “core”

This ledger-first design is chosen to avoid lock-in to ad-hoc undo paths and hidden agent actions.
The product’s trust surface (legibility, reversibility, witness-paced commits) should emanate from
this core rather than being bolted on later.
