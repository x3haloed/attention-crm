# Deals (Attention-First)

This doc defines how Deals work in Attention CRM beyond CRUD: the lifecycle, the default surfaces, and how “pipelines” fit without becoming taxonomy-first.

## Design invariants

- **Meaning before taxonomy:** users can capture value without knowing pipelines/stages.
- **No orphan work:** a deal must attach to ≥1 target entity (contact now; later any entity).
- **One stable attention anchor per surface:** Deal pages are “desks” (work surfaces), not dashboards.
- **Structure is a lens:** pipelines/stages are optional projections over deals, not required parentage.
- **Attention is the sorting key:** due next steps, staleness, and close windows drive what surfaces.

## Definition: what a Deal is

A Deal is a named *future outcome* with:
- `targets[]` (one or more entities)
- `title`
- `state` (`open|won|lost`)
- `next_step` (what + when; can be “none yet” but should be prompted)
- optional `value` and optional `close_window` (date or range)
- an activity timeline (notes/interactions/files later)

Everything else (stage, probability, forecast category, pipeline) must be *earned*, not required.

## Core surfaces

### 1) Universal Action Surface (omnibar)

Default behavior should bias toward “capture what happened”:
- Free text → suggest “Log note” (and attach to chosen targets)
- If the text implies an opportunity → also suggest “Create deal from this”

Power-user affordances (optional but recommended):
- `deal:` intent chip (create/attach deal)
- targets as chips (plain typing in deal mode should suggest entities)

### 2) Deal Desk (primary deal view)

The Deal page is a desk:
- Top: Deal title + state (open/won/lost)
- Next: **Next step** card (due time + action)
- Below: timeline (notes, calls, emails, meetings) and quick logging
- Peripheral: “Deal needs…” checklist (missing next step, missing value, missing targets, etc.)

The desk should make it easy to:
- add/update next step
- log an interaction
- attach/detach targets
- close won/lost

### 3) Pipeline Lens (secondary view)

Pipelines exist because CRMs need them, but they must behave like a lens:
- Default grouping: “milestone/stage” (optional label)
- Default ordering: attention (due/stale/closing), not purely stage order
- Drag/drop stage changes allowed (expert), never required (novice)

Stage is a reversible label; it must never block logging a note or setting a next step.

### 4) Needs Attention (homepage)

Deals should appear here when:
- next step is due or overdue
- deal is stale (no activity in N days)
- close window is soon and there is no next step

Each row should be actionable:
- “Mark next step done”
- “Set next step”
- “Open deal”

## Deal lifecycle (attention-first)

### Step 1: Capture
Most deals begin as a sentence. The system should:
- allow logging the note first (always)
- then optionally offer: “This sounds like a deal—create one?”

Creation should not demand:
- pipeline selection
- stage selection
- probability/value

### Step 2: Qualify (missing info prompts)
Instead of forcing stage fields, prompt for what’s missing:
- pick/confirm targets
- set a next step (who/what/when)
- optionally set value and close window

### Step 3: Work (next step + continuity)
The “next step” card is the attention anchor:
- if empty, it should be the strongest call-to-action
- if due, it should be visually urgent and easy to complete/roll forward

### Step 4: Close
Closing is a single decisive action:
- `Won` or `Lost`
- one lightweight prompt: “What happened?” (1 sentence)
- optional “reason” taxonomy later (deferred)

### Step 5: Continuity
After close, suggest the next meaningful thing:
- Won → “handoff / onboarding next step”
- Lost → “follow up in 90 days?” (optional)

## Stage without taxonomy-first pain

Stage is treated as a **milestone label** that can be:
- inferred (e.g., “proposal sent”)
- explicitly set by experts
- always editable/reversible
- never required for usefulness

## Membership model (targets)

Deals should attach to **one or more** entities (`targets[]`):
- MVP: contacts
- Next: company/account, and eventually any entity type

Targets should be editable from the Deal Desk (meaning-first membership).

## MVP approach (pragmatic)

If we need to ship incrementally:
1) Create Deal + Deal Desk with next-step + timeline (single contact target initially)
2) Needs Attention includes deals (due/stale rules)
3) Pipeline lens (simple stage labels + attention sorting)
4) Multi-target attachments (generalize from contact-only)

## Acceptance criteria

- A user can create and work a deal without ever seeing a pipeline.
- “Next step” is always the dominant action on a deal.
- Needs Attention reliably surfaces deals that require action.
- Pipeline view is useful for experts but never blocks core workflows.

