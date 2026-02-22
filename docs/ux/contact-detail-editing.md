# Contact Detail + Editing (Attention-First UX)

Goal: make the contact page primarily about **capturing context + follow-up**, while allowing edits without falling into long forms.

This spec is intended as a handoff to design and as an implementation target for:
- Contact edit UI + update timestamps
- Contact edit form + update persistence

## Core principles

1. One primary action: log what happened + set follow-up.
2. Progressive disclosure: optional fields are "Add ..." affordances, not a long form.
3. Just-in-time requirements: policies gate actions, not the entire entity.
4. Inline edits + autosave (or per-field save). Avoid global "Save" mode.
5. Destructive actions are de-emphasized (menu/danger zone).

## Page layout

### A) Header strip

Elements:
- Back link
- Contact name (inline editable)
- Secondary: small "Saved" / "Saving..." indicator
- Overflow menu: Delete contact, Export, etc.

No global "Save" button.

### B) Identity card (compact)

Shows (when present):
- Email
- Phone
- Company

Each row is inline-editable (click to edit) and supports:
- empty-state affordance: "Add email", "Add phone", "Add company"
- inline validation (email format, phone normalization)
- quick copy / click actions optional (copy/email/call)

### C) Optional fields (collapsed by default)

A small section: "Add more" with pills/rows:
- Add notes
- Add job title
- Add location
- Add LinkedIn

These are not required for MVP, but the pattern matters: "Add ..." expands to a single field editor.

### D) Interaction composer (primary focus)

Single composer (avoid duplicate "Add note" button plus input).

Fields:
- Type (defaults to `note`; optional quick toggle: note/call/email/meeting)
- Content (multiline, placeholder: "What happened? Next steps?")
- Follow-up (optional):
  - checkbox or inline control: "Follow up"
  - date/time picker

Primary action button:
- "Log" (or "Log interaction")

Secondary behavior:
- If follow-up is set, the interaction should appear in Needs Attention until completed.

### E) Interaction timeline

List of interactions (newest first):
- Icon/type, short title/first line, timestamp
- If due date exists and not complete: show due chip
- "Mark complete" for due follow-ups (inline)

## Editing behaviors

### Inline edit pattern

- Clicking a value turns it into an input.
- On blur or Enter: autosave.
- Escape cancels.
- While saving: show "Saving..." near header or inline spinner.
- On error: show inline error and keep the editor open.

### Autosave semantics

- Edits are persisted per-field to reduce "mode switching".
- For self-host / low-JS mode: per-field forms are OK; autosave can be simulated via "Save" within the inline editor.

## Policy gating (requirements checklist)

Policy requirement example: generating `report_x.pdf` requires `initial_contact_date`.

Pattern:
- The entity page does NOT force `initial_contact_date`.
- The action ("Generate report_x.pdf") shows a small "Requirements" box:
  - Initial contact date: missing (Add)
  - Company: present
- Clicking "Add" opens an inline editor for the missing field.
- After completion, the action proceeds.

This keeps the CRM attention-first while still enabling structured outputs.

## Data requirements (implementation notes)

- Contact fields: `name`, `email`, `phone`, `company`, `notes`, `created_at`, `updated_at`.
- `updated_at` should be touched on:
  - contact field edits
  - interaction creation and completion (already implemented)
- Interaction fields: `type`, `content`, `due_at`, `completed_at`, `created_at`.

## Acceptance criteria

- A user can edit name/email/phone/company without navigating to a separate "edit" page.
- No long-form "edit mode" with a global Save button.
- The interaction composer is the most visually prominent interactive element below the header.
- Follow-up due date is easy to set at time of logging.
- A policy-required missing field is requested only when the user attempts the gated action.

