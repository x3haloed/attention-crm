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

  function parseJSONOrThrow(raw, label) {
    const s = (raw || "").trim();
    if (!s) return {};
    try {
      return JSON.parse(s);
    } catch (e) {
      const err = new Error(`${label} must be valid JSON`);
      err.cause = e;
      throw err;
    }
  }

  async function sseFetch(url, opts, onEvent) {
    const res = await fetch(url, opts);
    if (!res.ok) {
      const text = await res.text().catch(() => "");
      throw new Error(`HTTP ${res.status}: ${text || "request failed"}`);
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
          if (!raw) continue;
          let data = null;
          try {
            data = JSON.parse(raw);
          } catch {
            data = raw;
          }
          onEvent({ type: currentEvent || "message", data });
        }
      }
    }
  }

  let inFlight = null;

  async function main() {
    const form = $("#infer-config-form");
    if (!form) return;

    const slug = tenantSlug();
    const status = $("#infer-config-status");
    const provider = $("#infer-provider");
    const baseURL = $("#infer-base-url");
    const model = $("#infer-model");
    const apiKey = $("#infer-api-key");
    const headers = $("#infer-headers");
    const output = $("#infer-stream-output");
    const clearBtn = $("#infer-stream-clear");
    const testBtn = $("#infer-stream-test");

    const csrf = window.attentionCsrfToken ? window.attentionCsrfToken() : "";
    const cfgURL = `/t/${slug}/agent/infer/config`;
    const streamURL = `/t/${slug}/agent/infer/stream`;

    form.addEventListener("submit", async (ev) => {
      ev.preventDefault();
      setStatus(status, "Saving…");
      try {
        const dto = {
          provider: (provider && provider.value ? provider.value : "").trim(),
          base_url: (baseURL && baseURL.value ? baseURL.value : "").trim(),
          model: (model && model.value ? model.value : "").trim(),
          api_key: (apiKey && apiKey.value ? apiKey.value : "").trim(),
          headers: parseJSONOrThrow(headers ? headers.value : "{}", "Extra headers"),
        };
        if (!dto.api_key) delete dto.api_key;

        const res = await fetch(cfgURL, {
          method: "POST",
          credentials: "same-origin",
          headers: {
            "Content-Type": "application/json",
            "X-CSRF-Token": csrf,
          },
          body: JSON.stringify(dto),
        });
        if (!res.ok) {
          const text = await res.text().catch(() => "");
          throw new Error(text || `HTTP ${res.status}`);
        }
        setStatus(status, "Saved.", "ok");
      } catch (e) {
        setStatus(status, e && e.message ? e.message : "Save failed.", "error");
      }
    });

    if (clearBtn && output) {
      clearBtn.addEventListener("click", () => {
        output.textContent = "";
      });
    }

    if (testBtn && output) {
      testBtn.addEventListener("click", async () => {
        output.textContent = "";
        setStatus(status, "Streaming…");

        if (inFlight) {
          try {
            inFlight.abort();
          } catch {}
          inFlight = null;
        }
        const controller = new AbortController();
        inFlight = controller;

        const req = {
          messages: [
            { role: "system", content: "You are a helpful assistant. Stream your response." },
            { role: "user", content: "Respond with a short sentence confirming streaming works." },
          ],
        };

        try {
          await sseFetch(
            streamURL,
            {
              method: "POST",
              credentials: "same-origin",
              headers: {
                "Content-Type": "application/json",
                "X-CSRF-Token": csrf,
              },
              body: JSON.stringify(req),
              signal: controller.signal,
            },
            (ev) => {
              if (ev.type === "response.output_text.delta" && ev.data && ev.data.delta) {
                output.textContent += String(ev.data.delta);
              } else if (ev.type === "error" && ev.data && ev.data.message) {
                output.textContent += `\n[error] ${ev.data.message}\n`;
              }
            },
          );
          setStatus(status, "Streaming complete.", "ok");
        } catch (e) {
          if (e && e.name === "AbortError") return;
          setStatus(status, e && e.message ? e.message : "Streaming failed.", "error");
        } finally {
          if (inFlight === controller) inFlight = null;
        }
      });
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", main);
  } else {
    main();
  }
})();

