#!/usr/bin/env python3
import argparse
import json
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable


STATUSES = ["backlog", "next", "doing", "blocked", "done", "rejected"]
PRIORITIES = ["low", "medium", "high", "urgent"]


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def read_jsonl(path: Path) -> Iterable[dict]:
    if not path.exists():
        return []
    with path.open("r", encoding="utf-8") as f:
        seq = 0
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
                if isinstance(obj, dict):
                    seq += 1
                    obj["_seq"] = seq
                    yield obj
            except json.JSONDecodeError:
                continue


@dataclass
class Card:
    task_id: str
    title: str
    status: str = "backlog"
    priority: str = "medium"
    updated_at: str = ""
    updated_seq: int = 0
    latest_snapshot_id: str | None = None
    provisional: bool = False


def set_updated(card: Card, ts: str, seq: int) -> None:
    if seq >= card.updated_seq:
        card.updated_seq = seq
        if ts:
            card.updated_at = ts


def fold(root: Path) -> dict[str, Any]:
    ledger_path = root / ".isnad" / "ledger.jsonl"
    control_path = root / ".isnad" / "control.jsonl"

    cards: dict[str, Card] = {}
    unread_directives: dict[str, list[str]] = {}
    acked_directives: set[str] = set()
    last_ack_directive_id: str | None = None
    last_ack_directive_ts: str | None = None
    last_ack_control_seq = 0

    # First pass: evidence defines tasks and receipts/snapshots.
    for rec in read_jsonl(ledger_path):
        rec_type = rec.get("type")
        ts = rec.get("ts", "")
        seq = int(rec.get("_seq", 0) or 0)
        task_id = rec.get("task_id")

        if rec_type == "task_opened" and isinstance(task_id, str) and task_id:
            title = ""
            meta = rec.get("meta") if isinstance(rec.get("meta"), dict) else {}
            if isinstance(meta, dict):
                title = meta.get("title") or ""
            if not title:
                title = rec.get("claim") or "Untitled task"
            cards[task_id] = Card(task_id=task_id, title=str(title), provisional=False)
            set_updated(cards[task_id], ts, seq)

        if rec_type in ("task_updated",) and isinstance(task_id, str) and task_id and task_id in cards:
            meta = rec.get("meta") if isinstance(rec.get("meta"), dict) else {}
            if isinstance(meta, dict) and meta.get("title"):
                cards[task_id].title = str(meta["title"])
            set_updated(cards[task_id], ts, seq)

        if rec_type == "snapshot" and isinstance(task_id, str) and task_id and task_id in cards:
            cards[task_id].latest_snapshot_id = rec.get("id")
            set_updated(cards[task_id], ts, seq)

        if rec_type == "ack_directive":
            meta = rec.get("meta") if isinstance(rec.get("meta"), dict) else {}
            directive_id = meta.get("directive_id") if isinstance(meta, dict) else None
            if isinstance(directive_id, str):
                acked_directives.add(directive_id)
                last_ack_directive_id = directive_id
                last_ack_directive_ts = ts or last_ack_directive_ts

    # Second pass: control directives steer status/priority and define unread set.
    for d in read_jsonl(control_path):
        d_id = d.get("id")
        d_type = d.get("type")
        ts = d.get("ts", "")
        seq = int(d.get("_seq", 0) or 0)
        task_id = d.get("task_id")
        payload = d.get("payload") if isinstance(d.get("payload"), dict) else {}

        if d_type == "open_task":
            if isinstance(task_id, str) and task_id:
                title = payload.get("title") if isinstance(payload, dict) else None
                if task_id not in cards:
                    cards[task_id] = Card(
                        task_id=task_id,
                        title=str(title or "Untitled task"),
                        provisional=True,
                    )
                else:
                    if title and cards[task_id].title in ("(unopened task)", "Untitled task"):
                        cards[task_id].title = str(title)
                status = payload.get("status")
                if status in STATUSES:
                    cards[task_id].status = status
                priority = payload.get("priority")
                if priority in PRIORITIES:
                    cards[task_id].priority = priority
                set_updated(cards[task_id], ts, seq)

        if isinstance(task_id, str) and task_id and task_id not in cards:
            # Steering unknown tasks is allowed but should look provisional.
            cards[task_id] = Card(task_id=task_id, title="(unopened task)", provisional=True)

        if isinstance(task_id, str) and task_id and task_id in cards:
            card = cards[task_id]

            if d_type == "set_status":
                status = payload.get("status")
                if status in STATUSES:
                    card.status = status
                    set_updated(card, ts, seq)

            if d_type == "set_priority":
                priority = payload.get("priority")
                if priority in PRIORITIES:
                    card.priority = priority
                    set_updated(card, ts, seq)

            if d_type == "pause":
                card.status = "blocked"
                set_updated(card, ts, seq)

            if isinstance(d_id, str) and d_id and d_id not in acked_directives:
                unread_directives.setdefault(task_id, []).append(d_id)
            elif isinstance(d_id, str) and d_id and d_id in acked_directives:
                last_ack_control_seq = max(last_ack_control_seq, seq)

    columns: dict[str, list[dict[str, Any]]] = {k: [] for k in STATUSES}
    cards_out: dict[str, dict[str, Any]] = {}

    for task_id, card in cards.items():
        card_dict = {
            "task_id": card.task_id,
            "title": card.title,
            "status": card.status,
            "priority": card.priority,
            "updated_at": card.updated_at,
            "updated_seq": card.updated_seq,
            "latest_snapshot_id": card.latest_snapshot_id,
            "unread_directive_count": len(unread_directives.get(task_id, [])),
            "provisional": card.provisional,
        }
        cards_out[task_id] = card_dict
        columns[card.status].append(card_dict)

    for status in STATUSES:
        columns[status].sort(
            key=lambda c: (c.get("priority", ""), int(c.get("updated_seq", 0) or 0)),
            reverse=True,
        )

    return {
        "generated_at": utc_now(),
        "columns": columns,
        "cards": cards_out,
        "unread_directives": unread_directives,
        "last_ack_directive_id": last_ack_directive_id,
        "last_ack_directive_ts": last_ack_directive_ts,
        "last_ack_control_seq": last_ack_control_seq,
    }


def render_markdown(board: dict[str, Any]) -> str:
    lines: list[str] = []
    lines.append("# Board (derived)")
    lines.append("")
    lines.append(f"Generated: {board.get('generated_at', '')}")
    lines.append("")

    for status in STATUSES:
        lines.append(f"## {status.capitalize()}")
        for card in board["columns"][status]:
            unread = card.get("unread_directive_count", 0)
            provisional = " (provisional)" if card.get("provisional") else ""
            suffix = f" (unread:{unread})" if unread else ""
            lines.append(f"- [{card['task_id']}] {card['title']}{provisional}  ({card['priority']}){suffix}")
        lines.append("")

    return "\n".join(lines).rstrip() + "\n"


def write_json(path: Path, obj: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Fold .isnad ledger/control into a derived board state.")
    parser.add_argument("--root", default=".", help="Repo root containing .isnad/")
    args = parser.parse_args()

    root = Path(args.root).expanduser().resolve()
    board = fold(root)

    out_dir = root / ".isnad" / "state"
    out_json = out_dir / "board.json"
    out_md = out_dir / "board.md"
    write_json(out_json, board)
    out_md.write_text(render_markdown(board), encoding="utf-8")

    print(f"[OK] Wrote {out_json}")
    print(f"[OK] Wrote {out_md}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
