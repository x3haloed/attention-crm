# Attention CRM — UX Manual (v1.0)

## Purpose
Attention CRM is designed to support trustworthy co‑agency between humans and AI in relationship management. The system prioritizes human attention, preserves agency, and enables progressive autonomy through visible, interruptible, and auditable agent actions.

---

## Core Principles

### 0. Meaning Before Taxonomy (Attention‑First Information Architecture)
- There must be exactly one primary object of meaning for the user’s task (in CRM: the relationship record — contact/account/deal context).
- Any hierarchy above the primary object is optional, associative, and reversible (membership, tags, collections), never a prerequisite to act.
- The system must not demand epistemic compliance before usefulness: users should not have to learn the taxonomy to perform their first task.
- Prefer recognition over recall: default views present the meaningful objects and their next actions, not the classification system.
- Taxonomy is earned and disclosed progressively, based on user intent stabilizing and need arising (e.g., reuse, reporting, delegation), not at creation time.
- Avoid taxonomy‑first attention hijack: classification should never steal focus from intent, capture, and follow‑through.
- Provide deterministic predictability: optional structure must be visible, navigable, and undoable without reframing the user’s world.
- Support messy real‑world membership: objects can belong to multiple groups/segments; membership changes are first‑class, auditable, and reversible.


### 1. Human‑First Workdesk
- The primary workspace centers on contacts, deals, notes, and tasks.
- Users can complete all core work without AI.
- AI augments workflows but never replaces the primary control surface.

### 2. Situated Agent (Stable Locus of Agency)
- The AI has a persistent, named workspace adjacent to the user’s desk.
- The agent is spatially and conceptually locatable.
- Users can always see where the agent “lives” and how to address it.

### 3. Co‑Agency, Not Automation
- The system models a collaboration between human and agent.
- Both can act on the same objects.
- All agent actions are attributable and reversible.

### 4. Trust Through Legibility
- Agent actions are expressed using the same verbs and objects as the UI.
- Users can understand what happened without learning a new abstraction.
- Decision provenance is available without exposing raw model reasoning.

### 5. Witness‑Paced Commits (Day‑1 Trust Calibration)
- Salient actions are paced at human‑legible speed before commit.
- The user can pause, intervene, or cancel before irreversible actions.
- This is synchronous supervision, not simulated playback.

### 6. Progressive Compression of Trust
- As trust grows, stepwise visibility compresses into summaries.
- Users can always expand actions into full detail.
- High‑risk actions retain explicit commit boundaries.

### 7. Interruptibility and Sovereignty
- A persistent Pause/Stop control allows the user to halt the agent.
- The agent never commits irreversible actions without an intervention window.
- Delegation is always reversible.

### 8. Objects Are the Locus of Work; Events Are the Locus of Trust
- Work happens on contacts, deals, and activities.
- Trust accumulates through a visible history of actions.
- The activity spine supports auditability without dominating attention.

---

## Locked‑In UX Decisions

### Layout Structure
- Center: Human work surface (contact/deal focus, quick actions, activity).
- Left: Attention panel (items requiring human focus).
- Right: Agent workspace (avatar, status, plans, activity, learning).
- Activity history accessible without dominating the interface.

### Agent Workspace (Avatar)
The agent workspace includes:
- Name and identity (e.g., “Alex AI”).
- Current status and autonomy level.
- Pause/Stop control.
- Monitoring scope.
- Planned actions.
- Recent activity.
- Learned preferences.

### Inline Attribution
- Agent actions are attributed in context (e.g., “Email sent by Alex AI”).
- Inline references avoid ambient badges and reduce cognitive noise.
- AI contributions appear where work happens, not as global clutter.

### Salient Action Boundaries
Salient actions include:
- Sending external communications.
- Scheduling meetings.
- Updating deal stages.
- Deleting or bulk editing data.

These actions:
- trigger witness pacing on Day‑1,
- provide pause and undo controls,
- may require explicit confirmation based on risk.

### Witness‑Paced Execution
- The agent may think instantly but commits actions at human‑legible pace.
- The UI presents actions in familiar form (e.g., draft appearing in editor).
- The user can interrupt before commit.

### Two‑Phase Commit for External Effects
- Actions affecting the outside world are staged before commit.
- Draft → Preview → Commit model ensures intervention points.

### Activity Spine (Trust Layer)
The activity spine provides:
- Human and agent actions.
- Expandable causal chains.
- Undo and revision controls.
- Decision provenance (reason, evidence, policy).

### Decision Provenance Format
Each action may include:
- Reason.
- Evidence.
- Policy applied.
- User involvement.

This ensures auditability without exposing raw chain‑of‑thought.

### Trust Calibration Progression

#### Day‑1: Supervised Execution
- Witness‑paced commits for salient actions.
- Inline attribution and visible plans.
- Frequent learning confirmations.

#### Day‑N: Assisted Execution
- Reduced pacing for medium‑risk actions.
- Summaries replace stepwise playback.
- High‑risk commits still gated.

#### Mature Trust: Delegated Execution
- Low‑risk actions auto‑commit.
- Exceptions and summaries highlighted.
- Pause/Stop remains available.

---

## Agent Behavior Contract

### The Agent Must:
- Expose planned actions before commit.
- Attribute its actions clearly.
- Provide undo and revision.
- Learn from user corrections.
- Respect commit boundaries.

### The Agent Must Not:
- Act invisibly.
- Commit irreversible actions without intervention.
- Present decisions without human‑legible reasoning.

---

## Human Experience Goals

Users should feel:
- In control, even when delegating.
- Able to follow and verify agent behavior.
- Confident that mistakes are recoverable.
- That the agent is a visible collaborator, not a hidden system.

---

## Design Litmus Tests

A design change is acceptable only if:
1. The human can see what the agent did.
2. The human can understand why.
3. The human can pause or undo.
4. The human could perform the action themselves.

---

## Summary
Attention CRM is a human‑first workdesk with a situated AI collaborator. Trust is built through legible actions, witness‑paced commits, and progressive autonomy. The system ensures that users remain sovereign while benefiting from AI assistance that becomes more efficient as trust grows.

