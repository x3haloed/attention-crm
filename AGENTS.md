## Wake Instructions (Isnad Ledger)

This repo uses an isnad-style provenance ledger + control directives under `.isnad/`.

On every new agent session (or after a context wipe), do this before making changes:

1. Read the current derived board:
   - `.isnad/state/board.md`
2. Read pending control directives:
   - `.isnad/control.jsonl`
3. If you act on any directive, append an evidence receipt record (append-only):
   - Use `python3 tools/work-board/scripts/append_ledger.py`
   - Record type: `ack_directive`
   - Include `meta.directive_id` with the directive id you are acknowledging.
4. After any steering or task update, regenerate the derived board:
   - `python3 tools/work-board/scripts/fold_state.py --root .`

Notes:
- Never edit or delete `.isnad/ledger.jsonl`. Correct mistakes by appending a superseding record.
- Never edit `.isnad/state/*` by hand (derived outputs).
