#!/usr/bin/env python3
import argparse
import json
import re
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def new_id() -> str:
    return f"D_{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}_{uuid4().hex[:12]}"

def new_task_id() -> str:
    return f"T_{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}_{uuid4().hex[:8]}"


def append_jsonl(path: Path, obj: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as f:
        f.write(json.dumps(obj, sort_keys=True) + "\n")


def parse_payload(payload: str) -> dict:
    try:
        return json.loads(payload)
    except json.JSONDecodeError as e:
        raise SystemExit(f"Invalid JSON payload: {e}") from e


TASK_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$")


def validate_task_id(task_id: str) -> None:
    if not TASK_ID_RE.match(task_id):
        raise SystemExit(
            "Invalid --task value. Use 1-64 chars of letters/digits/underscore/dash, starting with letter/digit."
        )


def main() -> int:
    parser = argparse.ArgumentParser(description="Append a human control directive to .isnad/control.jsonl.")
    parser.add_argument("--root", default=".", help="Repo root containing .isnad/")
    parser.add_argument("--type", required=True, help="Directive type (e.g., set_status, set_priority, set_goal).")
    parser.add_argument(
        "--task",
        dest="task_id",
        help="Task id. Required for task-scoped directives; optional for global directives; optional for open_task (auto-minted).",
    )
    parser.add_argument("--payload", default="{}", help="JSON payload string.")
    parser.add_argument("--rationale", default="", help="Short rationale for the directive.")
    parser.add_argument("--author", default="human", help="Directive author (default: human).")
    parser.add_argument("--meta", default="{}", help="JSON object for directive meta (e.g., via/operator/host).")
    args = parser.parse_args()

    root = Path(args.root).expanduser().resolve()
    control = root / ".isnad" / "control.jsonl"

    task_scoped_types = {"set_status", "set_priority", "pause", "resume", "note"}
    global_types = {"set_goal", "request_summary", "reject_record"}

    if args.type in task_scoped_types and not args.task_id:
        raise SystemExit(f"--task is required for --type {args.type}")

    if args.type == "open_task" and not args.task_id:
        args.task_id = new_task_id()

    if args.task_id:
        validate_task_id(args.task_id)

    try:
        meta = json.loads(args.meta)
        if not isinstance(meta, dict):
            raise ValueError("meta must be a JSON object")
    except Exception as e:
        raise SystemExit(f"Invalid --meta: {e}") from e

    directive = {
        "id": new_id(),
        "ts": utc_now(),
        "type": args.type,
        "author": args.author,
        "meta": meta,
        "payload": parse_payload(args.payload),
    }
    if args.task_id:
        directive["task_id"] = args.task_id
    if args.rationale:
        directive["rationale"] = args.rationale

    append_jsonl(control, directive)
    print(f"[OK] Appended directive {directive['id']} to {control}")
    if directive.get("type") == "open_task":
        print(f"     Task id: {directive.get('task_id')}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
