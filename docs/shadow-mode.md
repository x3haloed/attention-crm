# Shadow Mode (Observe-Only) + Hyper-Episodic Ropes

Shadow mode is the default agent operating mode for early Attention CRM:

- The agent **observes** what the human does (UI actions + ledger commits).
- The agent can **message** the user.
- The agent **cannot write** to the product state or perform external effects.

This doc locks down the core interaction contract for making the agent feel responsive while staying legible and auditable.

## Goals

- **Mutual legibility:** the human can understand what the agent saw and why it said something.
- **Low lock-in:** avoid complex long-running conversational state in the LLM.
- **Responsiveness:** fast feedback without streaming screenshots or keystrokes by default.
- **Safety:** shadow mode has no write capabilities.

## Pairing (User-Scoped Agent)

For now, an agent is paired to a **single human session within a tenant**.

Implications:

- The rope buffer and shadow-mode invocations are scoped to `(tenant, user-session)`.
- Multi-user shared-agent behavior is explicitly deferred until we have a clear collaboration model.

## Hyper-Episodic Execution Model

We embrace a **hyper-episodic** architecture: each agent invocation is a fresh episode whose context is provided via an **event rope**.

The LLM “conversation” does not accumulate. Every invocation is:

1. `system`: fixed policy + tool contract
2. `user`: a rope of relevant history (timestamped) + instructions
3. `assistant`: **tool calls only**

All assistant text outside `<tool_call>…</tool_call>` is **dropped**.

### Why

- Makes intent detection trivial (tools vs no tools).
- Makes audits trivial (each invocation is self-contained).
- Avoids hidden global state / “Redux time travel” problems.

## Event Rope

An event rope is a **timestamped bundle** of what has happened recently plus any user-entered chat messages.

## Salience (What Triggers an Invocation)

**Salient** means: *any system state transition or external effect caused.*

Non-salient examples:

- UI clicks, focus changes, or navigation that do not cause a state transition

**Trigger rule (v0):** every **ledger event** triggers a shadow-mode invocation.

External effects should still be modeled via ledger events (draft/preview/commit/observe), so the rule above covers them.

UI events may still be buffered as optional context, but they should not be the primary trigger mechanism.

### Sources

The rope can include events from:

- **UI semantics** (navigation, submits, errors, drafts; coarse and debounced)
- **Ledger commits** (authoritative durable state changes)
- **Agent events** (previous `agent.spine.event` entries)

### Human vocabulary (no op names)

Ropes must not speak in internal op names (e.g. `human.note`, `contact.field.set`).

Every event included in a rope must also have a **user-language narration line**, such as:

- “You wrote a note on John Doe…”
- “You changed John Doe’s email to …”
- “You logged a call with John Doe…”

The structured payload may include internal identifiers, but the narration is the shared language between human and agent.

### Rope buffer (server-side)

We maintain a server-side, append-only **workspace rope buffer** (per tenant + client session/tab).

Each invocation includes a **tail window** of that rope, not the entire history.

## Truncation + “Agent Remembers From Here” Marker

When the rope exceeds the prompt budget, we trim the oldest items and insert a marker that is surfaced in the UI:

> “Agent context starts here. Ask the agent to search history to recall earlier events.”

Trim order (drop first):

1. draft updates
2. low-salience UI events (focus/scroll/hover)
3. keep: commits, errors, navigations, and user chat messages as long as possible

The marker should include a stable reference (event id + timestamp) so it is auditable.

## Tools and Output Contract

The agent only affects the UI via tool calls.

Two UI tools are always available:

- `ui.no_action()`: explicit “I am choosing to do nothing.”
- `ui.message({text, spine?})`: display a message to the user (optionally also append a spine item).

### Shadow mode tool availability

Shadow mode allows:

- `ui.no_action`, `ui.message`
- read-only tools (queries, retrieval, lookups)

Shadow mode forbids:

- any write tool (mutations to contacts/interactions/deals)
- any external effect tool (email send, calendar, etc.)

This is enforced by **tool availability**, not by “trusting the model”.

## Agent Rail

The agent rail is **agent-only**:

- Shows what the agent is doing / has done (`agent.spine.event`).
- Human activity is visible in the Ledger view, not the rail.

## Responsiveness (UX)

Responsiveness comes from:

- small, frequent rope-driven invocations
- clear “NO_ACTION” fast-path via `ui.no_action()`
- rendering the agent’s visible output only when it intentionally calls `ui.message(...)`

## Open Questions (Deferred)

- How to represent “intent sessions” as markers inside the rope buffer (vs only a continuous stream).
- Whether/how to add an on-demand “screen context” fetch (view-model, accessibility-tree-lite, or screenshot) when needed.
- Exact salience triggers (which events should cause an agent invocation and which should only be buffered).
