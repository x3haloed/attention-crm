package app

import "html/template"

func setupFormHTML(errText, defaultWorkspace string) template.HTML {
	if defaultWorkspace == "" {
		defaultWorkspace = "Acme"
	}
	errBlock := ""
	if errText != "" {
		errBlock = `<div class="mb-4 bg-red-50 border border-red-200 rounded-lg p-3 text-sm text-red-800">` + template.HTMLEscapeString(errText) + `</div>`
	}
	return template.HTML(errBlock + `<form id="setup-passkey-form" method="POST" class="space-y-4">
<div>
  <label class="block text-sm font-medium text-gray-700">Workspace name</label>
  <input class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" name="workspace_name" value="` + template.HTMLEscapeString(defaultWorkspace) + `" required autocomplete="organization">
</div>
<div>
  <label class="block text-sm font-medium text-gray-700">Tenant slug</label>
  <input class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" name="tenant_slug" placeholder="acme" required autocomplete="off">
</div>
<div>
  <label class="block text-sm font-medium text-gray-700">Owner name</label>
  <input class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" name="name" required autocomplete="name">
</div>
<div>
  <label class="block text-sm font-medium text-gray-700">Owner email</label>
  <input class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" name="email" type="email" required autocomplete="email">
</div>
<button class="w-full bg-blue-600 text-white px-6 py-2.5 rounded-lg font-medium hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 text-sm" type="submit">Create workspace and enroll passkey</button>
</form>
<p id="setup-status" class="mt-4 text-sm text-gray-600"></p>
<script>
(function() {
  const form = document.getElementById("setup-passkey-form");
  const status = document.getElementById("setup-status");
  if (!form) return;

  const b64ToBuf = (b64) => Uint8Array.from(atob((b64 + "===".slice((b64.length + 3) % 4)).replace(/-/g, "+").replace(/_/g, "/")), c => c.charCodeAt(0));
  const bufToB64 = (buf) => btoa(String.fromCharCode(...new Uint8Array(buf))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
  const normalizeCreation = (pk) => {
    pk.challenge = b64ToBuf(pk.challenge);
    pk.user.id = b64ToBuf(pk.user.id);
    if (pk.excludeCredentials) pk.excludeCredentials = pk.excludeCredentials.map(c => ({...c, id: b64ToBuf(c.id)}));
    return pk;
  };
  const marshalCreation = (cred) => ({
    id: cred.id,
    rawId: bufToB64(cred.rawId),
    type: cred.type,
    response: {
      attestationObject: bufToB64(cred.response.attestationObject),
      clientDataJSON: bufToB64(cred.response.clientDataJSON),
      transports: cred.response.getTransports ? cred.response.getTransports() : []
    }
  });

  const buildFormData = (form) => {
    const fd = new FormData(form);
    // Some browsers can visually autofill without the value being present in FormData.
    // Force-set current DOM values as a safety net.
    form.querySelectorAll("input[name],select[name],textarea[name]").forEach((el) => {
      try { fd.set(el.name, el.value || ""); } catch (_) {}
    });
    return fd;
  };

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    status.className = "mt-4 text-sm text-gray-600";
    status.textContent = "Starting setup...";
    try {
      const startResp = await fetch("/setup/passkey/start", { method: "POST", body: buildFormData(form) });
      if (!startResp.ok) throw new Error(await startResp.text());
      const start = await startResp.json();
      const opts = start.options.publicKey || (start.options.response && start.options.response.publicKey) || start.options;
      const credential = await navigator.credentials.create({ publicKey: normalizeCreation(opts) });
      const finishResp = await fetch("/setup/passkey/finish?flow_id=" + encodeURIComponent(start.flow_id), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(marshalCreation(credential))
      });
      if (!finishResp.ok) throw new Error(await finishResp.text());
      const finish = await finishResp.json();
      window.location.href = finish.redirect;
    } catch (err) {
      status.className = "mt-4 text-sm text-red-700";
      status.textContent = "Setup failed: " + err.message;
    }
  });
})();
</script>
</div>`)
}

func loginFormHTML(slug, errText string) template.HTML {
	errBlock := ""
	if errText != "" {
		errBlock = `<div class="mb-4 bg-red-50 border border-red-200 rounded-lg p-3 text-sm text-red-800">` + template.HTMLEscapeString(errText) + `</div>`
	}
	return template.HTML(errBlock + `<div class="space-y-4">
<button id="login-discoverable-btn" class="w-full bg-blue-600 text-white px-6 py-2.5 rounded-lg font-medium hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 text-sm" type="button">Use passkey</button>
<div class="flex items-center gap-3">
  <div class="h-px bg-gray-200 flex-1"></div>
  <div class="text-xs text-gray-500 font-medium">or</div>
  <div class="h-px bg-gray-200 flex-1"></div>
</div>
<form id="login-passkey-form" method="POST" class="space-y-4">
<div>
  <label class="block text-sm font-medium text-gray-700">Email</label>
  <input class="mt-1 block w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500" name="email" type="email" required autocomplete="email">
</div>
<button class="w-full bg-blue-600 text-white px-6 py-2.5 rounded-lg font-medium hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 text-sm" type="submit">Sign in with passkey</button>
</form>
<p id="login-status" class="mt-4 text-sm text-gray-600"></p>
<script>
(function() {
  const form = document.getElementById("login-passkey-form");
  const discoverableBtn = document.getElementById("login-discoverable-btn");
  const status = document.getElementById("login-status");
  if (!form || !status) return;

  const b64ToBuf = (b64) => Uint8Array.from(atob((b64 + "===".slice((b64.length + 3) % 4)).replace(/-/g, "+").replace(/_/g, "/")), c => c.charCodeAt(0));
  const bufToB64 = (buf) => btoa(String.fromCharCode(...new Uint8Array(buf))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
  const normalizeAssertion = (pk) => {
    pk.challenge = b64ToBuf(pk.challenge);
    if (pk.allowCredentials) pk.allowCredentials = pk.allowCredentials.map(c => ({...c, id: b64ToBuf(c.id)}));
    return pk;
  };
  const marshalAssertion = (cred) => ({
    id: cred.id,
    rawId: bufToB64(cred.rawId),
    type: cred.type,
    response: {
      authenticatorData: bufToB64(cred.response.authenticatorData),
      clientDataJSON: bufToB64(cred.response.clientDataJSON),
      signature: bufToB64(cred.response.signature),
      userHandle: cred.response.userHandle ? bufToB64(cred.response.userHandle) : null
    }
  });

  const buildFormData = (form) => {
    const fd = new FormData(form);
    form.querySelectorAll("input[name],select[name],textarea[name]").forEach((el) => {
      try { fd.set(el.name, el.value || ""); } catch (_) {}
    });
    return fd;
  };

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    status.className = "mt-4 text-sm text-gray-600";
    status.textContent = "Starting passkey login...";
    try {
      const startResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/login/passkey/start", { method: "POST", body: buildFormData(form) });
      if (!startResp.ok) throw new Error(await startResp.text());
      const start = await startResp.json();
      const opts = start.options.publicKey || (start.options.response && start.options.response.publicKey) || start.options;
      const assertion = await navigator.credentials.get({ publicKey: normalizeAssertion(opts) });
      const finishResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/login/passkey/finish?flow_id=" + encodeURIComponent(start.flow_id), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(marshalAssertion(assertion))
      });
      if (!finishResp.ok) throw new Error(await finishResp.text());
      const finish = await finishResp.json();
      window.location.href = finish.redirect;
    } catch (err) {
      status.className = "mt-4 text-sm text-red-700";
      status.textContent = "Login failed: " + err.message;
    }
  });

  if(discoverableBtn){
    discoverableBtn.addEventListener("click", async () => {
      status.className = "mt-4 text-sm text-gray-600";
      status.textContent = "Starting passkey login...";
      try {
        const startResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/login/passkey/discoverable/start", { method: "POST" });
        if (!startResp.ok) throw new Error(await startResp.text());
        const start = await startResp.json();
        const opts = start.options.publicKey || (start.options.response && start.options.response.publicKey) || start.options;
        const assertion = await navigator.credentials.get({ publicKey: normalizeAssertion(opts) });
        const finishResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/login/passkey/discoverable/finish?flow_id=" + encodeURIComponent(start.flow_id), {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(marshalAssertion(assertion))
        });
        if (!finishResp.ok) throw new Error(await finishResp.text());
        const finish = await finishResp.json();
        window.location.href = finish.redirect;
      } catch (err) {
        status.className = "mt-4 text-sm text-red-700";
        status.textContent = "Login failed: " + err.message;
      }
    });
  }
})();
</script>`)
}

func inviteRedeemHTML(slug, email, token string) template.HTML {
	return template.HTML(`<p>You have been invited to join <code>` + template.HTMLEscapeString(slug) + `</code>.</p>
<form id="invite-passkey-form" method="POST">
<label>Email</label><input name="email" type="email" value="` + template.HTMLEscapeString(email) + `" readonly>
<label>Your name*</label><input name="name" required>
<button type="submit">Enroll passkey and join</button>
</form>
<p id="invite-status"></p>
<script>
(function() {
  const form = document.getElementById("invite-passkey-form");
  const status = document.getElementById("invite-status");
  if (!form) return;
  const token = "` + template.HTMLEscapeString(token) + `";

  const b64ToBuf = (b64) => Uint8Array.from(atob((b64 + "===".slice((b64.length + 3) % 4)).replace(/-/g, "+").replace(/_/g, "/")), c => c.charCodeAt(0));
  const bufToB64 = (buf) => btoa(String.fromCharCode(...new Uint8Array(buf))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
  const normalizeCreation = (pk) => {
    pk.challenge = b64ToBuf(pk.challenge);
    pk.user.id = b64ToBuf(pk.user.id);
    if (pk.excludeCredentials) pk.excludeCredentials = pk.excludeCredentials.map(c => ({...c, id: b64ToBuf(c.id)}));
    return pk;
  };
  const marshalCreation = (cred) => ({
    id: cred.id,
    rawId: bufToB64(cred.rawId),
    type: cred.type,
    response: {
      attestationObject: bufToB64(cred.response.attestationObject),
      clientDataJSON: bufToB64(cred.response.clientDataJSON),
      transports: cred.response.getTransports ? cred.response.getTransports() : []
    }
  });

  const buildFormData = (form) => {
    const fd = new FormData(form);
    form.querySelectorAll("input[name],select[name],textarea[name]").forEach((el) => {
      try { fd.set(el.name, el.value || ""); } catch (_) {}
    });
    return fd;
  };

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    status.textContent = "Starting enrollment...";
    try {
      const startResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/invite/" + encodeURIComponent(token) + "/passkey/start", { method: "POST", body: buildFormData(form) });
      if (!startResp.ok) throw new Error(await startResp.text());
      const start = await startResp.json();
      const opts = start.options.publicKey || (start.options.response && start.options.response.publicKey) || start.options;
      const credential = await navigator.credentials.create({ publicKey: normalizeCreation(opts) });
      const finishResp = await fetch("/t/` + template.HTMLEscapeString(slug) + `/invite/" + encodeURIComponent(token) + "/passkey/finish?flow_id=" + encodeURIComponent(start.flow_id), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(marshalCreation(credential))
      });
      if (!finishResp.ok) throw new Error(await finishResp.text());
      const finish = await finishResp.json();
      window.location.href = finish.redirect;
    } catch (err) {
      status.textContent = "Enrollment failed: " + err.message;
    }
  });
})();
</script>`)
}
