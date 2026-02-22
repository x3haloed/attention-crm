#!/usr/bin/env python3
import argparse
import json
import os
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def new_id(prefix: str) -> str:
    return f"{prefix}_{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}_{uuid4().hex[:12]}"


@dataclass(frozen=True)
class Paths:
    root: Path
    isnad_dir: Path
    ledger: Path
    control: Path
    state_dir: Path
    board_json: Path
    board_md: Path
    cursors: Path


def paths_for(root: Path) -> Paths:
    isnad_dir = root / ".isnad"
    state_dir = isnad_dir / "state"
    return Paths(
        root=root,
        isnad_dir=isnad_dir,
        ledger=isnad_dir / "ledger.jsonl",
        control=isnad_dir / "control.jsonl",
        state_dir=state_dir,
        board_json=state_dir / "board.json",
        board_md=state_dir / "board.md",
        cursors=state_dir / "cursors.json",
    )


def ensure_dir(path: Path) -> None:
    path.mkdir(parents=True, exist_ok=True)


def append_jsonl(path: Path, obj: dict) -> None:
    ensure_dir(path.parent)
    with path.open("a", encoding="utf-8") as f:
        f.write(json.dumps(obj, sort_keys=True) + "\n")


def write_json(path: Path, obj: dict) -> None:
    ensure_dir(path.parent)
    path.write_text(json.dumps(obj, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Create a minimal .isnad workspace in a repo.")
    parser.add_argument("root", help="Target repo root (creates .isnad/ within it).")
    parser.add_argument("--force", action="store_true", help="Overwrite derived state files if present.")
    args = parser.parse_args()

    root = Path(args.root).expanduser().resolve()
    p = paths_for(root)

    ensure_dir(p.state_dir)

    if not p.ledger.exists():
        append_jsonl(
            p.ledger,
            {
                "id": new_id("L"),
                "ts": utc_now(),
                "type": "init",
                "claim": "Initialized isnad workspace.",
                "action": "Created .isnad directory and initial state files.",
                "artifact": {"path": str(p.isnad_dir.relative_to(root))},
                "evidence": {"cwd": str(root)},
                "next_decision": "continue",
                "meta": {"scaffold_version": 1},
            },
        )

    if not p.control.exists():
        p.control.write_text("", encoding="utf-8")

    if args.force or (not p.board_json.exists()):
        write_json(
            p.board_json,
            {
                "generated_at": utc_now(),
                "columns": {k: [] for k in ["backlog", "next", "doing", "blocked", "done", "rejected"]},
                "cards": {},
                "unread_directives": {},
            },
        )

    if args.force or (not p.board_md.exists()):
        p.board_md.write_text(
            "# Board (derived)\n\nRun fold_state.py to regenerate.\n",
            encoding="utf-8",
        )

    if args.force or (not p.cursors.exists()):
        write_json(
            p.cursors,
            {
                "generated_at": utc_now(),
                "control_ack_cursor": None,
                "last_seen_control_seq": 0,
                "last_ack_control_seq": 0,
                "folded_control_bytes": 0,
                "folded_ledger_bytes": 0,
            },
        )

    print(f"[OK] Initialized {p.isnad_dir}")
    print(f"  - {p.ledger}")
    print(f"  - {p.control}")
    print(f"  - {p.board_json}")
    print(f"  - {p.board_md}")
    print(f"  - {p.cursors}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
