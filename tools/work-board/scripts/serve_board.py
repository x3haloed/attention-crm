#!/usr/bin/env python3
import argparse
import json
import threading
import webbrowser
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from typing import Any

from fold_state import fold


HTML = """<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width,initial-scale=1" />
    <title>Isnad Board (derived)</title>
    <style>
      :root { --bg:#0b1020; --panel:#121a33; --text:#e6e9f2; --muted:#a8b0c6; --card:#1a2550; --accent:#7aa2f7; --warn:#f7768e; }
      * { box-sizing: border-box; font-family: ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial; }
      body { margin:0; background:var(--bg); color:var(--text); }
      header { padding:12px 16px; border-bottom:1px solid rgba(255,255,255,.08); display:flex; gap:16px; align-items:center; }
      header .badge { padding:2px 8px; border:1px solid rgba(255,255,255,.15); border-radius:999px; color:var(--muted); font-size:12px; }
      header .warn { color:var(--warn); border-color: rgba(247,118,142,.4); }
      main { padding:16px; }
      .cols { display:grid; grid-template-columns: repeat(6, 1fr); gap:12px; }
      .col { background:var(--panel); border:1px solid rgba(255,255,255,.08); border-radius:12px; padding:10px; min-height: 60vh; }
      .col h2 { margin:0 0 8px 0; font-size:14px; color:var(--muted); text-transform: uppercase; letter-spacing: .08em; }
      .card { background:var(--card); border:1px solid rgba(255,255,255,.10); border-radius:10px; padding:10px; margin:8px 0; cursor:pointer; }
      .card small { color:var(--muted); display:block; margin-top:4px; }
      .row { display:flex; gap:10px; align-items:center; }
      .pill { padding:2px 8px; border-radius:999px; border:1px solid rgba(255,255,255,.18); font-size:12px; color:var(--muted); }
      .pill.unread { border-color: rgba(247,118,142,.55); color: var(--warn); }
      .toolbar { display:flex; gap:8px; margin-top:8px; }
      button { background:transparent; border:1px solid rgba(255,255,255,.18); color:var(--text); border-radius:10px; padding:8px 10px; cursor:pointer; }
      button.primary { border-color: rgba(122,162,247,.6); color: var(--accent); }
      input, select { background:transparent; color:var(--text); border:1px solid rgba(255,255,255,.18); border-radius:10px; padding:8px 10px; }
      dialog { background:var(--panel); color:var(--text); border:1px solid rgba(255,255,255,.18); border-radius:12px; padding:14px; width: min(720px, 92vw); }
      dialog::backdrop { background: rgba(0,0,0,.6); }
      .muted { color: var(--muted); }
      .kv { display:grid; grid-template-columns: 120px 1fr; gap:6px 10px; }
      .kv code { color: var(--accent); }
      .actions { display:flex; justify-content:flex-end; gap:8px; margin-top:10px; }
      a { color: var(--accent); }
    </style>
  </head>
  <body>
    <header>
      <strong>Isnad Board</strong>
      <span class="badge">Derived UI (does not edit ledger)</span>
      <span id="meta" class="badge"></span>
      <span id="pending" class="badge warn" style="display:none;"></span>
      <span style="margin-left:auto" class="badge muted">Auto-refresh: 2s</span>
    </header>
    <main>
      <div class="toolbar">
        <button class="primary" id="newTask">Create task</button>
        <button id="foldNow">Fold now</button>
      </div>
      <div style="height:10px"></div>
      <div id="cols" class="cols"></div>
    </main>

    <dialog id="taskDialog">
      <form method="dialog">
        <h3 id="taskTitle" style="margin:0 0 10px 0;"></h3>
        <div class="kv">
          <div class="muted">Task</div><div><code id="taskId"></code></div>
          <div class="muted">Status</div>
          <div>
            <select id="statusSel">
              <option value="backlog">backlog</option>
              <option value="next">next</option>
              <option value="doing">doing</option>
              <option value="blocked">blocked</option>
              <option value="done">done</option>
              <option value="rejected">rejected</option>
            </select>
          </div>
          <div class="muted">Priority</div>
          <div>
            <select id="prioSel">
              <option value="low">low</option>
              <option value="medium">medium</option>
              <option value="high">high</option>
              <option value="urgent">urgent</option>
            </select>
          </div>
          <div class="muted">Unread</div><div><span id="unreadCount"></span></div>
        </div>
        <div style="height:12px"></div>
        <label class="muted">Note (appends directive)</label>
        <input id="noteText" placeholder="Optional note…" style="width:100%; margin-top:6px" />
        <div class="actions">
          <button value="cancel">Close</button>
          <button id="save" class="primary" value="default">Save</button>
        </div>
      </form>
    </dialog>

    <dialog id="newTaskDialog">
      <form method="dialog">
        <h3 style="margin:0 0 10px 0;">Create task</h3>
        <label class="muted">Title</label>
        <input id="newTitle" placeholder="Short, verb-led title" style="width:100%; margin-top:6px" />
        <div style="height:10px"></div>
        <div class="row">
          <select id="newStatus">
            <option value="backlog">backlog</option>
            <option value="next">next</option>
            <option value="doing">doing</option>
            <option value="blocked">blocked</option>
          </select>
          <select id="newPrio">
            <option value="medium">medium</option>
            <option value="low">low</option>
            <option value="high">high</option>
            <option value="urgent">urgent</option>
          </select>
        </div>
        <div class="actions">
          <button value="cancel">Cancel</button>
          <button id="create" class="primary" value="default">Create</button>
        </div>
      </form>
    </dialog>

    <script>
      const colsEl = document.getElementById('cols');
      const metaEl = document.getElementById('meta');
      const pendingEl = document.getElementById('pending');
      const taskDialog = document.getElementById('taskDialog');
      const newTaskDialog = document.getElementById('newTaskDialog');

      let board = null;
      let selected = null;

      function pill(text, cls='pill') {
        const s = document.createElement('span');
        s.className = cls;
        s.textContent = text;
        return s;
      }

      function render() {
        if (!board) return;
        colsEl.innerHTML = '';

        const ack = board.last_ack_directive_id ? `last ack: ${board.last_ack_directive_id}` : 'last ack: (none)';
        metaEl.textContent = `fold: ${board.generated_at} • ${ack}`;

        const totalUnread = Object.values(board.unread_directives || {}).reduce((n, arr) => n + (arr?.length || 0), 0);
        if (totalUnread > 0) {
          pendingEl.style.display = 'inline-block';
          pendingEl.textContent = `pending directives: ${totalUnread}`;
        } else {
          pendingEl.style.display = 'none';
        }

        const statuses = ['backlog','next','doing','blocked','done','rejected'];
        for (const st of statuses) {
          const col = document.createElement('div');
          col.className = 'col';
          const h2 = document.createElement('h2');
          h2.textContent = st;
          col.appendChild(h2);

          const cards = (board.columns?.[st] || []);
          for (const c of cards) {
            const card = document.createElement('div');
            card.className = 'card';
            card.onclick = () => openTask(c.task_id);

            const top = document.createElement('div');
            top.className = 'row';
            top.appendChild(document.createTextNode(`[${c.task_id}] ${c.title}${c.provisional ? ' (provisional)' : ''}`));
            card.appendChild(top);

            const row = document.createElement('div');
            row.className = 'row';
            row.style.marginTop = '8px';
            row.appendChild(pill(c.priority));
            if (c.unread_directive_count) row.appendChild(pill(`unread:${c.unread_directive_count}`, 'pill unread'));
            card.appendChild(row);

            const sm = document.createElement('small');
            sm.textContent = c.updated_at ? `updated: ${c.updated_at}` : '';
            card.appendChild(sm);

            col.appendChild(card);
          }

          colsEl.appendChild(col);
        }
      }

      async function fetchBoard() {
        const res = await fetch('/api/board');
        board = await res.json();
        render();
      }

      async function postJson(url, body) {
        const res = await fetch(url, { method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify(body) });
        if (!res.ok) throw new Error(await res.text());
        return await res.json();
      }

      function openTask(taskId) {
        selected = board.cards[taskId];
        document.getElementById('taskTitle').textContent = selected.title;
        document.getElementById('taskId').textContent = selected.task_id;
        document.getElementById('statusSel').value = selected.status;
        document.getElementById('prioSel').value = selected.priority;
        document.getElementById('unreadCount').textContent = String(selected.unread_directive_count || 0);
        document.getElementById('noteText').value = '';
        taskDialog.showModal();
      }

      document.getElementById('save').addEventListener('click', async (e) => {
        e.preventDefault();
        if (!selected) return;
        const status = document.getElementById('statusSel').value;
        const priority = document.getElementById('prioSel').value;
        const note = document.getElementById('noteText').value.trim();
        await postJson('/api/directives', { type:'set_status', task_id:selected.task_id, payload:{status} });
        await postJson('/api/directives', { type:'set_priority', task_id:selected.task_id, payload:{priority} });
        if (note) await postJson('/api/directives', { type:'note', task_id:selected.task_id, payload:{text:note} });
        taskDialog.close();
        await fetchBoard();
      });

      document.getElementById('newTask').addEventListener('click', () => {
        document.getElementById('newTitle').value = '';
        document.getElementById('newStatus').value = 'backlog';
        document.getElementById('newPrio').value = 'medium';
        newTaskDialog.showModal();
      });

      document.getElementById('create').addEventListener('click', async (e) => {
        e.preventDefault();
        const title = document.getElementById('newTitle').value.trim();
        const status = document.getElementById('newStatus').value;
        const priority = document.getElementById('newPrio').value;
        const payload = { title: title || 'Untitled task', status, priority };
        await postJson('/api/open_task', { payload });
        newTaskDialog.close();
        await fetchBoard();
      });

      document.getElementById('foldNow').addEventListener('click', async () => {
        await fetchBoard();
      });

      fetchBoard();
      setInterval(fetchBoard, 2000);
    </script>
  </body>
</html>
"""


def read_jsonl(path: Path):
    if not path.exists():
        return []
    out = []
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
                out.append(obj)
    return out


def append_jsonl(path: Path, obj: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as f:
        f.write(json.dumps(obj, sort_keys=True) + "\n")


def utc_now() -> str:
    from datetime import datetime, timezone

    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def new_directive_id() -> str:
    from datetime import datetime, timezone
    from uuid import uuid4

    return f"D_{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}_{uuid4().hex[:12]}"


def new_task_id() -> str:
    from datetime import datetime, timezone
    from uuid import uuid4

    return f"T_{datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')}_{uuid4().hex[:8]}"


class BoardServer:
    def __init__(self, root: Path, author: str, via: str, operator: str | None):
        self.root = root
        self.author = author
        self.via = via
        self.operator = operator
        self.control = root / ".isnad" / "control.jsonl"
        self.ledger = root / ".isnad" / "ledger.jsonl"
        self._cache: dict[str, Any] | None = None
        self._cache_stat: tuple[int, float, int, float] | None = None  # (ledger_size, ledger_mtime, control_size, control_mtime)

    def ensure_isnad(self) -> None:
        (self.root / ".isnad" / "state").mkdir(parents=True, exist_ok=True)
        if not self.control.exists():
            self.control.write_text("", encoding="utf-8")
        if not self.ledger.exists():
            self.ledger.write_text("", encoding="utf-8")

    def _stats(self) -> tuple[int, float, int, float]:
        ls = self.ledger.stat() if self.ledger.exists() else None
        cs = self.control.stat() if self.control.exists() else None
        return (
            int(ls.st_size) if ls else 0,
            float(ls.st_mtime) if ls else 0.0,
            int(cs.st_size) if cs else 0,
            float(cs.st_mtime) if cs else 0.0,
        )

    def get_board(self) -> dict[str, Any]:
        self.ensure_isnad()
        stats = self._stats()
        if self._cache is not None and self._cache_stat == stats:
            return self._cache
        board = fold(self.root)
        self._cache = board
        self._cache_stat = stats
        return board

    def append_directive(self, d: dict[str, Any]) -> dict[str, Any]:
        self.ensure_isnad()
        d = dict(d)
        d.setdefault("id", new_directive_id())
        d.setdefault("ts", utc_now())
        d.setdefault("author", self.author)
        d.setdefault("meta", {})
        if isinstance(d["meta"], dict):
            d["meta"].setdefault("via", self.via)
            if self.operator:
                d["meta"].setdefault("operator", self.operator)
        append_jsonl(self.control, d)
        self._cache = None
        self._cache_stat = None
        return {"ok": True, "directive_id": d["id"], "task_id": d.get("task_id")}


def make_handler(server: BoardServer):
    class Handler(BaseHTTPRequestHandler):
        def _send(self, code: int, body: str, content_type: str = "text/plain; charset=utf-8"):
            data = body.encode("utf-8")
            self.send_response(code)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            if self.path == "/" or self.path.startswith("/index.html"):
                return self._send(200, HTML, "text/html; charset=utf-8")
            if self.path.startswith("/api/board"):
                board = server.get_board()
                return self._send(200, json.dumps(board), "application/json; charset=utf-8")
            return self._send(404, "not found")

        def do_POST(self):
            length = int(self.headers.get("Content-Length", "0") or "0")
            raw = self.rfile.read(length) if length else b"{}"
            try:
                payload = json.loads(raw.decode("utf-8") or "{}")
            except json.JSONDecodeError:
                return self._send(400, "invalid json")

            if self.path == "/api/open_task":
                task_id = new_task_id()
                directive = {
                    "type": "open_task",
                    "task_id": task_id,
                    "payload": payload.get("payload") if isinstance(payload, dict) else {},
                }
                res = server.append_directive(directive)
                return self._send(200, json.dumps(res), "application/json; charset=utf-8")

            if self.path == "/api/directives":
                if not isinstance(payload, dict):
                    return self._send(400, "payload must be object")
                d_type = payload.get("type")
                task_id = payload.get("task_id")
                d_payload = payload.get("payload", {})
                if not isinstance(d_payload, dict):
                    return self._send(400, "payload.payload must be object")
                if not isinstance(d_type, str) or not d_type:
                    return self._send(400, "missing type")
                if d_type in ("set_status", "set_priority", "pause", "resume", "note") and not isinstance(task_id, str):
                    return self._send(400, "missing task_id")
                res = server.append_directive({"type": d_type, "task_id": task_id, "payload": d_payload})
                return self._send(200, json.dumps(res), "application/json; charset=utf-8")

            return self._send(404, "not found")

        def log_message(self, fmt: str, *args):
            return

    return Handler


def main() -> int:
    parser = argparse.ArgumentParser(description="Serve a minimal local Isnad Board UI (derived; writes control only).")
    parser.add_argument("--root", default=".", help="Repo root containing .isnad/")
    parser.add_argument("--host", default="127.0.0.1", help="Bind host (default: 127.0.0.1).")
    parser.add_argument("--port", type=int, default=8787, help="Bind port (default: 8787).")
    parser.add_argument("--no-open", action="store_true", help="Do not auto-open browser.")
    parser.add_argument("--author", default="human", help="Directive author value to write (default: human).")
    parser.add_argument("--via", default="board-ui", help="Directive meta.via value (default: board-ui).")
    parser.add_argument("--operator", default="", help="Directive meta.operator value (optional).")
    args = parser.parse_args()

    root = Path(args.root).expanduser().resolve()
    server = BoardServer(root=root, author=args.author, via=args.via, operator=(args.operator or None))

    httpd = HTTPServer((args.host, args.port), make_handler(server))
    url = f"http://{args.host}:{args.port}/"
    print(f"[OK] Serving {url}")
    print("     Derived UI: writes control directives only; does not edit ledger.")

    if not args.no_open:
        threading.Timer(0.25, lambda: webbrowser.open(url)).start()

    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        httpd.server_close()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

