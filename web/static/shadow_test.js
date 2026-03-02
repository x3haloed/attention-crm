(() => {
  function $(sel) {
    return document.querySelector(sel);
  }

  function tenantSlug() {
    const body = document.body;
    const slug = body ? body.getAttribute("data-tenant-slug") : "";
    return (slug || "").trim();
  }

  function setStatus(el, msg, kind) {
    if (!el) return;
    el.textContent = msg || "";
    el.className = "text-sm text-gray-600";
    if (kind === "error") el.className = "text-sm text-red-700";
    if (kind === "ok") el.className = "text-sm text-green-700";
  }

  async function fetchJSON(url, opts) {
    const res = await fetch(url, opts);
    if (!res.ok) {
      const text = await res.text().catch(() => "");
      throw new Error(text || `HTTP ${res.status}`);
    }
    return res.json();
  }

  async function sseFetch(url, opts, onEvent) {
    const res = await fetch(url, opts);
    if (!res.ok) {
      const text = await res.text().catch(() => "");
      throw new Error(text || `HTTP ${res.status}`);
    }
    const reader = res.body && res.body.getReader ? res.body.getReader() : null;
    if (!reader) throw new Error("stream not readable");

    const decoder = new TextDecoder();
    let buffer = "";
    let currentEvent = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });

      while (true) {
        const sep = buffer.indexOf("\n");
        if (sep === -1) break;
        const line = buffer.slice(0, sep);
        buffer = buffer.slice(sep + 1);

        const trimmed = line.trimEnd();
        if (!trimmed) {
          currentEvent = "";
          continue;
        }
        if (trimmed.startsWith("event:")) {
          currentEvent = trimmed.slice("event:".length).trim();
          continue;
        }
        if (trimmed.startsWith("data:")) {
          const raw = trimmed.slice("data:".length).trim();
          let data = null;
          try {
            data = raw ? JSON.parse(raw) : null;
          } catch {
            data = raw;
          }
          onEvent({ type: currentEvent || "message", data });
        }
      }
    }
  }

  async function main() {
    const runBtn = $("#shadow-run");
    if (!runBtn) return;

    const slug = tenantSlug();
    const csrf = window.attentionCsrfToken ? window.attentionCsrfToken() : "";
    const status = $("#shadow-status");
    const backfillStatus = $("#shadow-backfill-status");
    const force = $("#shadow-force");
    const trace = $("#shadow-trace");
    const rope = $("#shadow-rope");
    const clear = $("#shadow-clear");
    const refreshRope = $("#shadow-refresh-rope");
    const backfillBtn = $("#shadow-backfill");

    const ropeURL = `/t/${slug}/agent/rope`;
    const runURL = `/t/${slug}/agent/shadow/run`;
    const backfillURL = `/t/${slug}/agent/ledger/backfill`;

    async function loadRope() {
      if (!rope) return;
      try {
        const data = await fetchJSON(ropeURL, { credentials: "same-origin" });
        if (data && data.items == null) data.items = [];
        rope.textContent = JSON.stringify(data, null, 2);
      } catch (e) {
        rope.textContent = `[error] ${e && e.message ? e.message : "Failed to load rope"}`;
      }
    }

    if (refreshRope) refreshRope.addEventListener("click", loadRope);
    if (clear && trace) clear.addEventListener("click", () => (trace.textContent = ""));

    await loadRope();

    if (backfillBtn) {
      backfillBtn.addEventListener("click", async () => {
        setStatus(backfillStatus, "Backfilling…");
        try {
          const data = await fetchJSON(backfillURL, {
            method: "POST",
            credentials: "same-origin",
            headers: {
              "Content-Type": "application/json",
              "X-CSRF-Token": csrf,
            },
            body: JSON.stringify({}),
          });
          setStatus(backfillStatus, `Backfilled. ${JSON.stringify(data.result || {}, null, 0)}`, "ok");
          await loadRope();
        } catch (e) {
          setStatus(backfillStatus, e && e.message ? e.message : "Backfill failed.", "error");
        }
      });
    }

    runBtn.addEventListener("click", async () => {
      if (trace) trace.textContent = "";
      setStatus(status, "Running…");
      try {
        const lines = [];
        await sseFetch(
          runURL,
          {
            method: "POST",
            credentials: "same-origin",
            headers: {
              "Content-Type": "application/json",
              "X-CSRF-Token": csrf,
            },
            body: JSON.stringify({ force: !!(force && force.checked) }),
          },
          (ev) => {
            lines.push({ event: ev.type, data: ev.data });
            if (trace) trace.textContent = JSON.stringify(lines, null, 2);
            if (ev.type === "shadow.error") setStatus(status, ev.data && ev.data.message ? ev.data.message : "Error.", "error");
            if (ev.type === "shadow.skip") setStatus(status, ev.data && ev.data.message ? ev.data.message : "Skipped.", "error");
            if (ev.type === "shadow.done") setStatus(status, "Done.", "ok");
          },
        );
        await loadRope();
      } catch (e) {
        setStatus(status, e && e.message ? e.message : "Run failed.", "error");
      }
    });
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", main);
  } else {
    main();
  }
})();
