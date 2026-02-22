#!/usr/bin/env python3
import argparse
import json
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def new_id() -> str:
    return f"L_{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}_{uuid4().hex[:12]}"


def append_jsonl(path: Path, obj: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as f:
        f.write(json.dumps(obj, sort_keys=True) + "\n")


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Append an evidence record to .isnad/ledger.jsonl (reference implementation)."
    )
    parser.add_argument("--root", default=".", help="Repo root containing .isnad/")
    parser.add_argument("--type", required=True, help="Record type (e.g., task_opened, action, test_run).")
    parser.add_argument("--topic", default="", help="Topic/workstream tag (optional).")
    parser.add_argument("--task", dest="task_id", default="", help="Task id (optional).")
    parser.add_argument("--claim", default="", help="What you believe happened.")
    parser.add_argument("--action", default="", help="What you did.")
    parser.add_argument("--artifact", default="", help="Artifact pointer (string).")
    parser.add_argument("--evidence", default="", help="Evidence pointer (string).")
    parser.add_argument("--next", dest="next_decision", default="", help="Next decision (continue/escalate/close).")
    parser.add_argument("--meta", default="{}", help="JSON object for meta.")
    args = parser.parse_args()

    root = Path(args.root).expanduser().resolve()
    ledger = root / ".isnad" / "ledger.jsonl"

    try:
        meta = json.loads(args.meta)
        if not isinstance(meta, dict):
            raise ValueError("meta must be a JSON object")
    except Exception as e:
        raise SystemExit(f"Invalid --meta: {e}") from e

    record: dict = {
        "id": new_id(),
        "ts": utc_now(),
        "type": args.type,
        "meta": meta,
    }
    if args.topic:
        record["topic"] = args.topic
    if args.task_id:
        record["task_id"] = args.task_id
    if args.claim:
        record["claim"] = args.claim
    if args.action:
        record["action"] = args.action
    if args.artifact:
        record["artifact"] = args.artifact
    if args.evidence:
        record["evidence"] = args.evidence
    if args.next_decision:
        record["next_decision"] = args.next_decision

    append_jsonl(ledger, record)
    print(f"[OK] Appended record {record['id']} to {ledger}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

