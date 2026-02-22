#!/usr/bin/env python3
"""Append ack_directive evidence records for unacked control directives.

This keeps the evidence plane auditable without requiring manual bookkeeping.

- Reads `.isnad/control.jsonl` for directive ids
- Reads `.isnad/ledger.jsonl` for existing `ack_directive` receipts
- Appends `ack_directive` records for any directive ids not yet acknowledged

It does not interpret directive semantics; it only records that they were read.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from pathlib import Path

VENDORED_DIR = (Path(__file__).resolve().parent.parent / "tools" / "work-board" / "scripts").resolve()
APPEND_LEDGER = VENDORED_DIR / "append_ledger.py"


def read_jsonl(path: Path) -> list[dict]:
    if not path.exists():
        return []
    out: list[dict] = []
    with path.open("r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
                if isinstance(obj, dict):
                    out.append(obj)
            except json.JSONDecodeError:
                continue
    return out


def main() -> int:
    parser = argparse.ArgumentParser(description="Append ack_directive evidence records for unread directives")
    parser.add_argument("--root", default=".", help="Repo root")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    root = Path(args.root).expanduser().resolve()
    control = root / ".isnad" / "control.jsonl"
    ledger = root / ".isnad" / "ledger.jsonl"

    directives = [d for d in read_jsonl(control) if isinstance(d.get("id"), str) and d.get("id")]
    acked: set[str] = set()
    for rec in read_jsonl(ledger):
        if rec.get("type") != "ack_directive":
            continue
        meta = rec.get("meta") if isinstance(rec.get("meta"), dict) else {}
        did = meta.get("directive_id") if isinstance(meta, dict) else None
        if isinstance(did, str) and did:
            acked.add(did)

    todo = [d["id"] for d in directives if d["id"] not in acked]
    if not todo:
        print("[OK] No unacked directives")
        return 0

    actor = os.environ.get("ISNAD_ACK_ACTOR") or os.environ.get("USER") or "agent"
    for did in todo:
        if args.dry_run:
            print(f"[DRY] would ack {did}")
            continue
        cmd = [
            sys.executable,
            str(APPEND_LEDGER),
            "--root",
            str(root),
            "--type",
            "ack_directive",
            "--claim",
            f"Acknowledged directive {did}.",
            "--action",
            "Marked directive as read.",
            "--next",
            "continue",
            "--meta",
            json.dumps({"directive_id": did, "ack_actor": actor, "via": "scripts/isnad_ack.py"}),
        ]
        subprocess.check_call(cmd)
        print(f"[OK] acked {did}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
