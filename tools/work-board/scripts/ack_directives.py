#!/usr/bin/env python3
import argparse
import json
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def new_ledger_id() -> str:
    return f"L_{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}_{uuid4().hex[:12]}"


def read_jsonl(path: Path):
    if not path.exists():
        return
    with path.open("r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(obj, dict):
                yield obj


def append_jsonl(path: Path, obj: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as f:
        f.write(json.dumps(obj, sort_keys=True) + "\n")


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Append ack_directive receipts in the ledger for unread directives in control.jsonl."
    )
    parser.add_argument("--root", default=".", help="Repo root containing .isnad/")
    parser.add_argument("--limit", type=int, default=0, help="Max directives to ack (0 = no limit).")
    parser.add_argument("--actor", default="agent", help="Receipt actor identity string.")
    parser.add_argument("--dry-run", action="store_true", help="Print what would be acknowledged without writing.")
    args = parser.parse_args()

    root = Path(args.root).expanduser().resolve()
    ledger = root / ".isnad" / "ledger.jsonl"
    control = root / ".isnad" / "control.jsonl"

    acked: set[str] = set()
    for rec in read_jsonl(ledger) or []:
        if rec.get("type") != "ack_directive":
            continue
        meta = rec.get("meta") if isinstance(rec.get("meta"), dict) else {}
        directive_id = meta.get("directive_id") if isinstance(meta, dict) else None
        if isinstance(directive_id, str):
            acked.add(directive_id)

    to_ack: list[dict] = []
    for d in read_jsonl(control) or []:
        d_id = d.get("id")
        if not isinstance(d_id, str) or not d_id or d_id in acked:
            continue
        to_ack.append(d)

    if args.limit and args.limit > 0:
        to_ack = to_ack[: args.limit]

    if not to_ack:
        print("[OK] No unread directives found.")
        return 0

    for d in to_ack:
        d_id = d["id"]
        receipt = {
            "id": new_ledger_id(),
            "ts": utc_now(),
            "type": "ack_directive",
            "task_id": d.get("task_id"),
            "claim": f"Acknowledged directive {d_id}.",
            "action": "Recorded receipt of human intent; will follow up with actions/tests or cannot_comply.",
            "evidence": {"control_id": d_id},
            "next_decision": "continue",
            "meta": {"directive_id": d_id, "ack_actor": args.actor},
        }
        if args.dry_run:
            print(json.dumps(receipt, indent=2, sort_keys=True))
        else:
            append_jsonl(ledger, receipt)
            print(f"[OK] acked {d_id} -> {receipt['id']}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())

