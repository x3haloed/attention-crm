(() => {
  function getTenantSlug() {
    const body = document.body;
    const slug = body ? body.getAttribute("data-tenant-slug") : "";
    return (slug || "").trim();
  }

  function isModifiedClick(event) {
    return event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
  }

  function shouldInterceptAnchor(a, url, tenantSlug) {
    if (!a || !url || !tenantSlug) return false;
    if (a.hasAttribute("download")) return false;
    if (a.getAttribute("target") && a.getAttribute("target") !== "_self") return false;
    if (a.getAttribute("rel") && a.getAttribute("rel").includes("external")) return false;
    if (url.origin !== window.location.origin) return false;
    if (url.hash && url.pathname === window.location.pathname && url.search === window.location.search) return false;

    const base = `/t/${tenantSlug}/`;
    if (!url.pathname.startsWith(base)) return false;

    // Let file downloads and non-HTML endpoints do full navigation.
    if (url.pathname.endsWith(".csv")) return false;
    if (url.pathname.includes("/export/") && !url.pathname.endsWith("/export")) return false;

    return true;
  }

  function parseDeskMain(doc) {
    const root = doc.querySelector("#desk-root");
    if (!root) return null;
    const main = root.querySelector("main");
    return main || null;
  }

  function parseDeskTitle(doc) {
    const root = doc.querySelector("#desk-root");
    const t = root ? root.getAttribute("data-attention-title") : "";
    if (t && t.trim()) return t.trim();
    const titleEl = doc.querySelector("title");
    return titleEl && titleEl.textContent ? titleEl.textContent.trim() : "";
  }

  let inFlight = null;
  function navigateTo(href, opts) {
    opts = opts || {};
    const tenantSlug = getTenantSlug();
    const url = new URL(href, window.location.href);
    if (!tenantSlug) return window.location.assign(url.href);

    if (inFlight) {
      try {
        inFlight.abort();
      } catch {}
      inFlight = null;
    }
    const controller = new AbortController();
    inFlight = controller;

    if (window.attentionDesk && window.attentionDesk.setLoading) {
      window.attentionDesk.noteNavHref(url.href);
      window.attentionDesk.setLoading(true, { href: url.href });
    }

    return fetch(url.href, {
      method: "GET",
      credentials: "same-origin",
      headers: { Accept: "text/html", "X-Attention-Fragment": "desk-root" },
      signal: controller.signal,
    })
      .then((res) => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        return res.text().then((text) => ({ text, finalURL: res.url || url.href }));
      })
      .then(({ text, finalURL }) => {
        const parser = new DOMParser();
        const doc = parser.parseFromString(text, "text/html");
        const newMain = parseDeskMain(doc);
        if (!newMain) throw new Error("Missing desk content.");

        const currentRoot = document.querySelector("#desk-root");
        const currentMain = currentRoot ? currentRoot.querySelector("main") : null;
        if (!currentMain) throw new Error("Missing current desk root.");

        const imported = document.importNode(newMain, true);
        currentMain.replaceWith(imported);

        const newTitle = parseDeskTitle(doc);
        if (newTitle) document.title = newTitle;

        const nextHref = finalURL || url.href;
        if (opts.replace) {
          history.replaceState({ href: nextHref }, "", nextHref);
        } else {
          history.pushState({ href: nextHref }, "", nextHref);
        }

        window.dispatchEvent(new Event("attention:desk:swap"));

        if (url.hash) {
          const id = url.hash.slice(1);
          const el = id ? document.getElementById(id) : null;
          if (el) el.scrollIntoView({ block: "start" });
        } else if (!opts.preserveScroll) {
          window.scrollTo(0, 0);
        }
      })
      .catch((err) => {
        if (err && err.name === "AbortError") return;
        if (window.attentionDesk && window.attentionDesk.setError) {
          window.attentionDesk.setError(err && err.message ? err.message : "Load failed.", url.href);
          return;
        }
        window.location.assign(url.href);
      })
      .finally(() => {
        if (inFlight === controller) inFlight = null;
      });
  }

  document.addEventListener(
    "click",
    (event) => {
      if (event.defaultPrevented) return;
      if (event.button !== 0) return;
      if (isModifiedClick(event)) return;

      const a = event.target && event.target.closest ? event.target.closest("a") : null;
      if (!a) return;
      const href = a.getAttribute("href") || "";
      if (!href || href.startsWith("javascript:")) return;

      const tenantSlug = getTenantSlug();
      const url = new URL(a.href, window.location.href);
      if (!shouldInterceptAnchor(a, url, tenantSlug)) return;

      event.preventDefault();
      navigateTo(url.href, { replace: false, preserveScroll: false });
    },
    true,
  );

  window.addEventListener("popstate", () => {
    navigateTo(window.location.href, { replace: true, preserveScroll: true });
  });
})();

