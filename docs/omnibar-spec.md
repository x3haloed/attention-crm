# Omnibar UX Spec (Unified, Attention-First)

This document governs how the omnibar behaves across the app. It defines a single unified workflow for search + “do something” actions via **chips** (confirmed tokens), a **palette** (suggestions), and an explicit **preview action** (what Enter would do now).

## Principles

- **Attention-first:** keep the user in a single focused surface (omnibar) for quick capture.
- **Explicit execution:** never “auto-create” or “auto-log” on Enter unless the user has explicitly selected/confirmed the action.
- **No orphan notes:** a note must be attached to at least one entity (contact now; “any entity” as the model expands).
- **Power-user fluent:** fast keyboard paths (type → `:`/Tab/Enter) without forcing “command syntax”.
- **Progressive disclosure:** plain typing works; chips/keywords unlock more power.

## Concepts

### Intent
What the user is trying to do (examples): `search`, `note`, `call`, `email`, `meeting`, `create_contact`.

### Slots
Structured values collected by the omnibar (as chips):
- `intent` chip: e.g. `Note`, `Call`
- `targets[]` chips: one or more entity references (e.g. `Bob Smith`, later: `Acme Corp`, `Deal: Q1 Renewal`)
- `content` (free text in the input)
- `due_at` (optional; could become a chip later)

### Chips
Small “tokens” above/inside the omnibar that represent confirmed values (intent or targets). Chips are removable.

### Palette rows
The dropdown list under the omnibar. Rows can represent:
- entity suggestions (contacts, later other entities)
- action suggestions (log note with X, create contact, pick entity, etc.)
- mode suggestions (commit `Note` chip, commit `Call` chip, etc.)

### Preview action
One palette row is the “preview” (highlighted). Enter triggers it.

## State machine

### State A: Free typing (default)
- **No chips**.
- Typing shows palette:
  - entity matches (contacts now)
  - action matches (e.g. create contact)
  - mode suggestions (e.g. `Note mode`)
- If text appears “note-like”, also show note logging paths (see “Note inference”).

### State B: Intent locked (Note/Call/Email/Meeting chip present)
- The omnibar expects either:
  1) a **target** entity, or
  2) **content** (note text), or
  3) both in any order.
- In this state, plain `Bob Smith` MUST be allowed to resolve as a target (no `contact:` required).

### State C: Target capture (one or more target chips present)
- With intent chip present, palette prioritizes:
  - additional targets (to add more)
  - “submit action” once the content is non-empty
- With intent chip present but **no targets**, submission is blocked: Enter should lead to “Pick entity…” (not log).

### State D: Ready to submit
Ready when:
- intent chip exists, AND
- `targets[]` has at least one entity, AND
- content is non-empty (trimmed).

Enter logs the interaction attached to the selected targets.

## Keyboard & mouse behavior

### Palette open/close
- Palette opens on input changes when there are suggestions.
- `Esc` closes palette.

### Selection + execution
- `ArrowUp/ArrowDown`: move preview row.
- `Enter`: execute preview row.
- `Tab`: commit the preview row **without executing** if it represents a chip-able value (intent or target). If it is an action row, Tab behaves like Enter.
- Click: same as Enter on that row.

### Chip creation shortcuts
- Typing an intent keyword followed by `:` commits the intent chip immediately:
  - `note:` → `Note` chip
  - `call:` → `Call` chip
  - `email:` → `Email` chip
  - `meet:` or `meeting:` → `Meeting` chip
- After committing an intent chip, the input remains focused and empty (or continues with remaining text after the colon).

### Chip deletion
- `Backspace`:
  - If input has text: normal backspace.
  - If input is empty: remove the most recent chip (target first, then intent).

## Note inference (when to suggest “note” paths)

The omnibar should treat input as note-like when any are true:
- 4+ words, OR
- contains verbs like `mentioned`, `discussed`, `said`, `follow up`, `remind`, OR
- contains due-time hints (`today`, `tomorrow`) or similar.

When note-like and no intent chip exists:
- palette should include a “Note mode” row (commit `Note` chip).
- palette should include quick “log note” suggestions (see below), but execution still requires explicit selection.

## Palette rows (required)

### 1) Intent chip rows
Shown when the user types an intent keyword prefix:
- “Note mode” (chip)
- “Call mode” (chip)
- “Email mode” (chip)
- “Meeting mode” (chip)

### 2) Entity match rows (targets)
Shown when the input could be a target:
- show contacts now; later show any entity type.
- selecting commits a **target chip** (not a log action by itself).

### 3) “Log note with …” action rows (2 + pick)
When input is note-like and we have target matches:
- Show **two** action rows:
  - “Log note with {TopMatch1}”
  - “Log note with {TopMatch2}”
- Third row: “Pick entity…” (see below)

When input is note-like and we do NOT have target matches:
- Show:
  - “Pick entity…” (primary)
  - optionally “Search contacts…” (secondary)

### 4) “Pick entity…” action row (the resolver)
This row switches the omnibar into a target-selection flow:
- If no intent chip exists, it should first commit `Note` chip (or offer to).
- It should focus entity suggestions and treat subsequent typing as “target search”.
- Selecting an entity commits a target chip and returns to normal input for content.

### 5) Create contact action row
When the input looks like a contact name:
- show “Create contact: {text}”
- selecting creates the contact (explicit) and then:
  - either commits it as a target chip (if intent mode is active), or
  - navigates to the new contact (if in plain search mode).

## Submission rules (no orphan notes)

Logging an interaction requires:
- intent chip present, AND
- at least one target entity chip present.

If the user hits Enter while in intent mode without targets:
- the preview action should be “Pick entity…” (never silently choose a target).

## Attachment model (forward-looking requirement)

Notes/interactions should be attachable to **one or more entities** (contacts now, later any entity type).

Near-term implementation note (MVP):
- current data model attaches interactions to a single contact.
- the omnibar should still be implemented to *feel* like it’s choosing a target entity; later we generalize storage to multi-target.

## Acceptance criteria (UX)

- Typing a sentence like “Bob suggested that we could …” reliably surfaces a note path (never only “Create contact …”).
- In note-like scenarios, palette shows 2 “log note with …” rows + a “Pick entity…” row.
- `note:` commits Note chip; then typing “Bob Smith” can commit a contact chip without `contact:`.
- With `Note` + `Bob Smith` chips, the remaining input is the note body; Enter logs it.

## Non-goals (for now)

- Free-form natural language parsing into multiple entities automatically.
- Deal objects / pipelines (can be added later).
- Multi-org discovery inside the omnibar (handled by `/t/{slug}` routing today).

