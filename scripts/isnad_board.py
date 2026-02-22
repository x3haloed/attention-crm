#!/usr/bin/env python3
"""Minimal local board UI for .isnad.

- Renders derived board (by running fold_state.py on each page load)
- Allows writing directives (open_task, set_status, set_priority)
- Never edits ledger.jsonl

No third-party dependencies.
"""

from __future__ import annotations

import argparse
import html
import json
import os
import subprocess
import sys
import urllib.parse
import webbrowser
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

VENDORED_DIR = (Path(__file__).resolve().parent.parent / "tools" / "work-board" / "scripts").resolve()
APPEND_DIRECTIVE = VENDORED_DIR / "append_directive.py"
FOLD_STATE = VENDORED_DIR / "fold_state.py"


@dataclass(frozen=True)
class RepoPaths:
    root: Path
    isnad: Path
    control: Path
    board_json: Path


def paths_for(root: Path) -> RepoPaths:
    isnad = root / ".isnad"
    return RepoPaths(
        root=root,
        isnad=isnad,
        control=isnad / "control.jsonl",
        board_json=isnad / "state" / "board.json",
    )


def run_fold(root: Path) -> None:
    if not FOLD_STATE.exists():
        raise RuntimeError(f"fold_state.py not found at {FOLD_STATE}")
    subprocess.check_call([sys.executable, str(FOLD_STATE), "--root", str(root)])


def read_board(board_json: Path) -> dict[str, Any]:
    return json.loads(board_json.read_text(encoding="utf-8"))


def append_directive(root: Path, directive_type: str, task_id: str | None, payload: dict[str, Any]) -> None:
    if not APPEND_DIRECTIVE.exists():
        raise RuntimeError(f"append_directive.py not found at {APPEND_DIRECTIVE}")

    author = os.environ.get("ISNAD_AUTHOR", "human")
    cmd = [
        sys.executable,
        str(APPEND_DIRECTIVE),
        "--root",
        str(root),
        "--type",
        directive_type,
        "--author",
        author,
        "--payload",
        json.dumps(payload),
    ]
    if task_id:
        cmd.extend(["--task", task_id])
    subprocess.check_call(cmd)


def html_page(title: str, body: str) -> bytes:
    css = """
    body{font-family:ui-sans-serif,system-ui;margin:18px;background:#f6f7f9}
    h1{margin:0 0 12px}
    .wrap{max-width:1200px;margin:0 auto}
    .row{display:flex;gap:12px;align-items:flex-start;flex-wrap:wrap}
    .col{background:#fff;border:1px solid #e4e6eb;border-radius:10px;padding:10px;min-width:260px;flex:1}
    .col h2{margin:0 0 8px;font-size:16px}
    .card{border:1px solid #eceff3;border-radius:10px;padding:10px;margin:8px 0;background:#fcfcfd}
    .meta{color:#666;font-size:12px;margin-top:6px}
    label{display:block;font-size:12px;color:#333;margin:6px 0 2px}
    input,select,button{font-size:13px;padding:8px;border-radius:8px;border:1px solid #d7dbe3}
    input,select{width:100%;box-sizing:border-box}
    button{background:#111;color:#fff;border-color:#111;cursor:pointer}
    button.secondary{background:#fff;color:#111}
    .bar{display:flex;gap:8px;align-items:flex-end;flex-wrap:wrap}
    .bar form{display:flex;gap:8px;align-items:flex-end;flex-wrap:wrap}
    .bar .field{min-width:240px}
    .notice{background:#fff3cd;border:1px solid #ffeeba;padding:10px;border-radius:10px;margin:10px 0}
    code{background:#f1f3f6;padding:2px 6px;border-radius:6px}
    """
    out = f"<!doctype html><html><head><meta charset='utf-8'><meta name='viewport' content='width=device-width, initial-scale=1'><title>{html.escape(title)}</title><style>{css}</style></head><body><div class='wrap'>{body}</div></body></html>"
    return out.encode("utf-8")


class Handler(BaseHTTPRequestHandler):
    server_version = "isnad-board/0.1"

    def do_GET(self) -> None:
        try:
            run_fold(self.server.repo.root)
            board = read_board(self.server.repo.board_json)
            page = render_board(board)
            self._send(200, html_page("Isnad Board", page), "text/html; charset=utf-8")
        except Exception as e:
            self._send(500, html_page("Error", f"<pre>{html.escape(str(e))}</pre>"), "text/html; charset=utf-8")

    def do_POST(self) -> None:
        try:
            length = int(self.headers.get("Content-Length", "0") or "0")
            raw = self.rfile.read(length)
            form = urllib.parse.parse_qs(raw.decode("utf-8"), keep_blank_values=True)

            action = (form.get("action") or [""])[0]
            operator = os.environ.get("ISNAD_OPERATOR") or os.environ.get("USER") or "human"
            via = "board-ui"

            if action == "open_task":
                title = (form.get("title") or [""])[0].strip()
                status = (form.get("status") or ["backlog"])[0]
                priority = (form.get("priority") or ["medium"])[0]
                if not title:
                    raise ValueError("title required")
                append_directive(
                    self.server.repo.root,
                    "open_task",
                    None,
                    {"title": title, "status": status, "priority": priority, "meta": {"via": via, "operator": operator}},
                )
            elif action == "set_status":
                task_id = (form.get("task_id") or [""])[0]
                status = (form.get("status") or [""])[0]
                if not task_id or not status:
                    raise ValueError("task_id and status required")
                append_directive(
                    self.server.repo.root,
                    "set_status",
                    task_id,
                    {"status": status, "meta": {"via": via, "operator": operator}},
                )
            elif action == "set_priority":
                task_id = (form.get("task_id") or [""])[0]
                priority = (form.get("priority") or [""])[0]
                if not task_id or not priority:
                    raise ValueError("task_id and priority required")
                append_directive(
                    self.server.repo.root,
                    "set_priority",
                    task_id,
                    {"priority": priority, "meta": {"via": via, "operator": operator}},
                )
            else:
                raise ValueError("unknown action")

            self.send_response(303)
            self.send_header("Location", "/")
            self.end_headers()
        except Exception as e:
            self._send(400, html_page("Error", f"<div class='notice'><pre>{html.escape(str(e))}</pre></div><p><a href='/'>Back</a></p>"), "text/html; charset=utf-8")

    def log_message(self, format: str, *args: Any) -> None:
        return

    def _send(self, code: int, data: bytes, content_type: str) -> None:
        self.send_response(code)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def render_board(board: dict[str, Any]) -> str:
    generated = html.escape(str(board.get("generated_at", "")))

    open_task_form = """
    <div class='bar'>
      <form method='POST'>
        <input type='hidden' name='action' value='open_task'>
        <div class='field'>
          <label>New task</label>
          <input name='title' placeholder='What should happen next?' required>
        </div>
        <div class='field'>
          <label>Status</label>
          <select name='status'>
            <option value='backlog'>backlog</option>
            <option value='next' selected>next</option>
            <option value='doing'>doing</option>
            <option value='blocked'>blocked</option>
            <option value='done'>done</option>
            <option value='rejected'>rejected</option>
          </select>
        </div>
        <div class='field'>
          <label>Priority</label>
          <select name='priority'>
            <option value='low'>low</option>
            <option value='medium' selected>medium</option>
            <option value='high'>high</option>
            <option value='urgent'>urgent</option>
          </select>
        </div>
        <div class='field'>
          <label>&nbsp;</label>
          <button type='submit'>Open task</button>
        </div>
      </form>
    </div>
    """

    html_out = [
        f"<h1>Board</h1>",
        f"<div class='meta'>Generated: <code>{generated}</code></div>",
        open_task_form,
        "<div class='row'>",
    ]

    columns = board.get("columns", {})
    order = ["backlog", "next", "doing", "blocked", "done", "rejected"]
    for col in order:
        cards = columns.get(col) or []
        html_out.append("<div class='col'>")
        html_out.append(f"<h2>{html.escape(col.capitalize())} ({len(cards)})</h2>")
        for c in cards:
            task_id = html.escape(str(c.get("task_id", "")))
            title = html.escape(str(c.get("title", "")))
            priority = html.escape(str(c.get("priority", "")))
            unread = int(c.get("unread_directive_count", 0) or 0)
            prov = " provisional" if c.get("provisional") else ""

            html_out.append("<div class='card'>")
            html_out.append(f"<div><strong>{title}</strong></div>")
            html_out.append(f"<div class='meta'><code>{task_id}</code> | {priority}{prov}{' | unread:'+str(unread) if unread else ''}</div>")

            html_out.append("<form method='POST' style='margin-top:8px'>")
            html_out.append("<input type='hidden' name='action' value='set_status'>")
            html_out.append(f"<input type='hidden' name='task_id' value='{task_id}'>")
            html_out.append("<label>Status</label>")
            html_out.append("<select name='status'>" + "".join(
                f"<option value='{s}' {'selected' if s==col else ''}>{s}</option>" for s in order
            ) + "</select>")
            html_out.append("<button class='secondary' type='submit' style='margin-top:6px'>Set status</button>")
            html_out.append("</form>")

            html_out.append("<form method='POST' style='margin-top:8px'>")
            html_out.append("<input type='hidden' name='action' value='set_priority'>")
            html_out.append(f"<input type='hidden' name='task_id' value='{task_id}'>")
            html_out.append("<label>Priority</label>")
            prios = ["low", "medium", "high", "urgent"]
            html_out.append("<select name='priority'>" + "".join(
                f"<option value='{p}' {'selected' if p==priority else ''}>{p}</option>" for p in prios
            ) + "</select>")
            html_out.append("<button class='secondary' type='submit' style='margin-top:6px'>Set priority</button>")
            html_out.append("</form>")

            html_out.append("</div>")

        html_out.append("</div>")

    html_out.append("</div>")
    return "\n".join(html_out)


def main() -> int:
    parser = argparse.ArgumentParser(description="Local web UI for .isnad board")
    parser.add_argument("--root", default=".", help="Repo root")
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8765)
    parser.add_argument("--no-open", action="store_true", help="Do not auto-open browser")
    args = parser.parse_args()

    root = Path(args.root).expanduser().resolve()
    repo = paths_for(root)
    if not repo.isnad.exists():
        raise SystemExit(".isnad/ not found; run scaffold first")

    server = ThreadingHTTPServer((args.host, args.port), Handler)
    server.repo = repo  # type: ignore[attr-defined]

    url = f"http://{args.host}:{args.port}/"
    print(f"[OK] Board UI at {url}")
    if not args.no_open:
        webbrowser.open(url)
    server.serve_forever()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
