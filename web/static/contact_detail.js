(() => {
  function qs(sel, root) {
    return (root || document).querySelector(sel);
  }

  function setSaveIndicator(saveIndicator, state) {
    if (!saveIndicator) return;
    if (state === "saving") {
      saveIndicator.innerHTML =
        '<svg class="w-4 h-4 text-gray-400 animate-spin" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 2a10 10 0 0 1 7.07 2.93l-1.41 1.41A8 8 0 1 0 20 12h2A10 10 0 0 1 12 22 10 10 0 0 1 12 2Z"/></svg><span class="text-gray-500">Saving...</span>';
      return;
    }
    if (state === "error") {
      saveIndicator.innerHTML =
        '<svg class="w-4 h-4 text-red-500" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20Zm1 13h-2v2h2v-2Zm0-10h-2v8h2V5Z"/></svg><span class="text-red-600">Not saved</span>';
      return;
    }
    saveIndicator.innerHTML =
      '<svg class="w-4 h-4 text-green-500" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4Z"/></svg><span class="text-green-600">Saved</span>';
  }

  function parseTenantSlug() {
    const body = document.body;
    const slug = body ? body.getAttribute("data-tenant-slug") : "";
    return (slug || "").trim();
  }

  function parseContactID() {
    const m = String(window.location.pathname || "").match(/\/contacts\/(\d+)/);
    if (!m) return 0;
    const n = Number(m[1]);
    return Number.isFinite(n) ? n : 0;
  }

  function initContactDetail() {
    const contactName = qs("#contact-name");
    const saveIndicator = qs("#save-indicator");
    if (!contactName || !saveIndicator) return;
    if (contactName.dataset.attentionInit === "1") return;
    contactName.dataset.attentionInit = "1";

    const tenantSlug = parseTenantSlug();
    const contactID = parseContactID();
    if (!tenantSlug || !contactID) return;
    const updateURL = `/t/${tenantSlug}/contacts/${contactID}/update`;

    const timers = new Map();
    function scheduleSave(el) {
      const field = el.getAttribute("data-field") || "";
      if (!field) return;
      const value = el.value || "";
      setSaveIndicator(saveIndicator, "saving");
      if (timers.has(field)) window.clearTimeout(timers.get(field));
      const t = window.setTimeout(() => doSave(field, value), 450);
      timers.set(field, t);
    }

    function doSave(field, value) {
      return fetch(updateURL, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-CSRF-Token": window.attentionCsrfToken ? window.attentionCsrfToken() : "",
        },
        body: JSON.stringify({ field, value }),
      })
        .then((res) => {
          if (!res.ok) throw new Error("bad status");
          return res.json();
        })
        .then(() => {
          setSaveIndicator(saveIndicator, "saved");
        })
        .catch(() => {
          setSaveIndicator(saveIndicator, "error");
        });
    }

    const fields = document.querySelectorAll("[data-field]");
    fields.forEach((el) => {
      if (el.dataset.attentionBound === "1") return;
      el.dataset.attentionBound = "1";
      el.addEventListener("input", () => scheduleSave(el));
      el.addEventListener("blur", () => {
        const field = el.getAttribute("data-field") || "";
        if (field && timers.has(field)) {
          window.clearTimeout(timers.get(field));
          timers.delete(field);
        }
        doSave(field, el.value || "");
      });
    });

    const addMore = qs("#add-more-btn");
    const optional = qs("#optional-fields");
    if (addMore && optional && addMore.dataset.attentionBound !== "1") {
      addMore.dataset.attentionBound = "1";
      addMore.addEventListener("click", () => optional.classList.toggle("hidden"));
    }

    const toggle = qs("#follow-up-toggle");
    const container = qs("#follow-up-date-container");
    const dueInput = qs("#follow-up-due-at");
    function syncFollowup() {
      if (!toggle || !container) return;
      const on = !!toggle.checked;
      container.classList.toggle("hidden", !on);
      if (dueInput) {
        dueInput.disabled = !on;
        dueInput.required = on;
        if (!on) dueInput.value = "";
      }
    }
    if (toggle && container && toggle.dataset.attentionBound !== "1") {
      toggle.dataset.attentionBound = "1";
      syncFollowup();
      toggle.addEventListener("change", syncFollowup);
    }
  }

  window.addEventListener("DOMContentLoaded", initContactDetail);
  window.addEventListener("attention:desk:swap", initContactDetail);
})();

