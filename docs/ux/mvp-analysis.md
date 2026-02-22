# 1️⃣ Homepage “Desk” UX Brief (for a UI designer)

## Purpose

Design a homepage that serves as a **work surface**, not a dashboard.

The page must allow users to:

* continue work
* find things
* capture new information
* orient themselves

It must **not** require users to understand system taxonomy before acting.

---

## Core UX invariants (non-negotiable)

1. There is one primary attention anchor.
2. The user can act immediately without selecting object types.
3. Structure is discoverable but not required.
4. Recognition is prioritized over recall.
5. All elements justify their demand for attention.

---

## Required functional regions (layout-agnostic)

### 🔹 1. Universal Action Surface (Primary Anchor)

**Purpose:** Allow users to retrieve or capture information without declaring intent.

**Behavior:**

* Typing searches existing entities (contacts, companies, deals, notes).
* Typing new text enables creation actions (note, contact, interaction).
* Suggestions appear inline.
* User can click suggestions or commit new entry.

**Must support:**

* Fuzzy search
* Inline creation
* Recognition-based suggestions
* No mode switching

**Example behaviors:**

* “Sarah” → shows Sarah Johnson
* “Call Sarah tomorrow” → create note or log call
* “Acme” → open company

---

### 🔹 2. Needs Attention (Actionable Focus)

**Purpose:** Surface items that require user action.

**Includes:**

* Follow-ups due
* Deals closing soon
* Unread messages
* Tasks assigned

**Constraints:**

* Must be actionable
* No vanity metrics
* No aggregate KPIs

---

### 🔹 3. Recent Interactions (Contextual Grounding)

**Purpose:** Help users resume work and maintain continuity.

**Includes:**

* Recent contacts interacted with
* Deals updated
* Notes created

**Behavior:**

* Clickable
* Ordered by recency
* Lightweight preview

---

### 🔹 4. Quick Capture Affordances (Visible, Optional)

**Purpose:** Provide recognition-based entry points for common actions.

**Examples:**

* New contact
* Log call
* Quick note

**Constraints:**

* Must not require object selection beforehand
* Must open minimal capture UI
* Must not dominate the page

---

## Elements explicitly excluded from homepage

To preserve attention alignment, do not include:

* Full pipeline dashboards
* KPI walls
* Mandatory filters
* Object-type navigation as primary entry
* Configuration panels
* Taxonomy-heavy controls

These belong deeper in the system.

---

## Behavioral progression (novice → expert)

The homepage must support:

### Novice

* Click quick actions
* Use recent items
* Ignore search bar

### Intermediate

* Use search to retrieve contacts
* Start typing to create notes

### Expert

* Use universal input for all actions
* Navigate entirely via search

The UI must support all three simultaneously.

---

## Success criteria

A new user can:

* log a call
* find a contact
* add a note

…within 10 seconds, without learning terminology.

---

# 2️⃣ CRM MVP Specification — “Skateboard”

## Goal

Deliver a fully usable CRM that supports core workflows with minimal structure.

The MVP must:

* support real work
* avoid taxonomy-first friction
* be extensible later

This is not the full car.
This is the skateboard: simple, fast, functional.

---

# MVP Scope: Core Capabilities

## Primary object of meaning: **Contact**

Contacts anchor the system.

Everything else is associative.

---

## Entities (minimal)

### 1. Contact

**Fields:**

* name (required)
* email
* phone
* company
* notes
* created_at
* updated_at

---

### 2. Interaction

Represents a note, call, email, or meeting.

**Fields:**

* contact_id
* type (note, call, email, meeting)
* content (text)
* due_date (optional)
* created_at

---

### 3. Deal (optional in skateboard, but recommended)

**Fields:**

* contact_id
* title
* value
* status (open, won, lost)
* close_date (optional)

---

## Core Workflows

### Workflow 1: Capture a new contact

**Entry points:**

* universal input
* quick action button

**Flow:**

1. User enters name
2. System checks for duplicates
3. User optionally adds email/phone/company
4. Save

**Constraints:**

* Only name required
* No required categories

---

### Workflow 2: Log an interaction

**Entry points:**

* contact page
* universal input
* quick action

**Flow:**

1. User selects or creates contact
2. Enters note or call summary
3. Optional follow-up date
4. Save

---

### Workflow 3: Find a contact

**Entry points:**

* universal input
* recent interactions list

**Behavior:**

* fuzzy search
* open contact detail

---

### Workflow 4: Follow-up reminders

**Behavior:**

* interactions with due_date appear in “Needs Attention”
* user can mark complete

---

## Homepage behavior (MVP version)

### Must include:

* universal action surface
* needs attention list
* recent interactions
* quick capture affordances

### Must not include:

* pipelines
* reporting
* dashboards
* segmentation
* automation

---

## Contact Detail Page

### Must show:

* contact info
* interaction timeline
* add interaction
* edit contact

### Must not require:

* categories
* tags
* pipelines

---

## Universal Input Behavior (MVP)

### Input handling rules:

If input matches existing contact → show results
If input resembles note → offer “Create note”
If input resembles name → offer “Create contact”

### Examples:

Typing:
“Sarah” → show Sarah Johnson
“Call Sarah tomorrow” → create note + due date suggestion
“Mike Chen” → create contact

---

## Data model invariants

1. Contacts exist independently.
2. Interactions attach to contacts.
3. Deals (if included) attach to contacts.
4. No required hierarchy.
5. No required tags.

---

## What this MVP intentionally does NOT solve

To keep the skateboard simple:

* pipelines
* teams
* permissions
* segmentation
* campaigns
* reporting
* automation
* complex taxonomy

These come later (bicycle → car).

---

# Why this MVP works

It supports the real task:

> “I talked to someone. I need to remember and follow up.”

Without forcing users to understand:

* leads vs contacts
* pipelines
* stages
* account hierarchies

It preserves attention and momentum.

---

# MVP Success Test

A new user can:

* Add a contact
* Log a call
* Set a follow-up
* Find that contact later

…without reading documentation or learning system terminology.

If they can’t, the skateboard is broken.

---

# Where this grows next (Bicycle)

When ready, you can add:

* tags (membership)
* deals pipeline
* team collaboration
* reporting
* automation

But the skateboard must remain intact underneath.