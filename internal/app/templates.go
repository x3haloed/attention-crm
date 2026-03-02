package app

const rootPageTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Attention CRM</title>
  <link rel="stylesheet" href="/static/tailwind.css">
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap" rel="stylesheet">
</head>
<body class="bg-gray-50 font-sans">
  <div class="min-h-screen">
    <div class="max-w-2xl mx-auto px-6 py-10">
      <div class="flex items-center space-x-2">
        <div class="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center">
          <span class="text-white text-sm font-semibold">A</span>
        </div>
        <span class="text-xl font-semibold text-gray-900">Attention CRM</span>
      </div>
      <div class="mt-8 bg-white rounded-2xl shadow-sm border border-gray-200 p-6">
        {{ if eq .TenantCount 0 }}
          <p class="text-sm text-gray-700">No workspace is configured yet.</p>
          <p class="mt-4"><a class="text-sm font-medium text-blue-600 hover:text-blue-700 hover:underline" href="/setup">Run initial setup</a></p>
        {{ else }}
          <p class="text-sm text-gray-700">Tenants exist. Open a tenant login URL directly, for example: <code class="px-2 py-1 rounded bg-gray-50 border border-gray-200">/t/my-org/login</code>.</p>
        {{ end }}
      </div>
    </div>
  </div>
</body>
</html>`

const tenantBaseTemplate = `{{define "page"}}<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  {{if .CSRFToken}}<meta name="attention-csrf" content="{{.CSRFToken}}">{{end}}
  <link rel="stylesheet" href="/static/tailwind.css">
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap" rel="stylesheet">
  <style>
    @keyframes agent-typing { from { width: 0; } to { width: 100%; } }
    @keyframes agent-caret { 0%, 50% { border-color: transparent; } 51%, 100% { border-color: rgb(59 130 246); } }
    @keyframes agent-pulse-glow {
      0% { box-shadow: 0 0 0 0 rgba(34,197,94,0.35); }
      70% { box-shadow: 0 0 0 10px rgba(34,197,94,0); }
      100% { box-shadow: 0 0 0 0 rgba(34,197,94,0); }
    }
    .agent-typing-animation {
      display: inline-block;
      max-width: 100%;
      white-space: nowrap;
      overflow: hidden;
      border-right: 2px solid rgb(59 130 246);
      animation: agent-typing 3s steps(40, end) infinite, agent-caret 1s step-end infinite;
    }
    .agent-typing-paused { animation-play-state: paused; border-right-color: transparent; }
    .agent-pulse-glow { animation: agent-pulse-glow 1.6s infinite; }
    @media (prefers-reduced-motion: reduce) {
      .agent-typing-animation { animation: none; border-right-color: transparent; }
      .agent-pulse-glow { animation: none; }
    }
  </style>
  <script>
    window.attentionCsrfToken = function () {
      var m = document.querySelector('meta[name="attention-csrf"]');
      return m ? (m.getAttribute("content") || "") : "";
    };
  </script>
  <script>
    // Desk loading UX (used by fragment navigation; harmless without it).
    (function () {
      function $(sel) { return document.querySelector(sel); }
      var slowTimer = null;
      var lastNavHref = "";

      function setDeskLoading(loading, opts) {
        opts = opts || {};
        var root = $("#desk-root");
        if (!root) return;
        var overlay = $("#desk-loading-overlay");
        if (!overlay) return;

        if (!loading) {
          root.classList.remove("attention-desk-loading");
          overlay.classList.add("hidden");
          var slow = $("#desk-slow-indicator");
          if (slow) slow.classList.add("hidden");
          var err = $("#desk-error");
          if (err) err.classList.add("hidden");
          if (slowTimer) { clearTimeout(slowTimer); slowTimer = null; }
          return;
        }

        lastNavHref = (opts.href || lastNavHref || "");
        var retry = $("#desk-retry");
        if (retry) retry.setAttribute("href", lastNavHref || window.location.href);

        root.classList.add("attention-desk-loading");
        overlay.classList.remove("hidden");

        var slow = $("#desk-slow-indicator");
        if (slow) slow.classList.add("hidden");
        if (slowTimer) clearTimeout(slowTimer);
        slowTimer = setTimeout(function () {
          var slow2 = $("#desk-slow-indicator");
          if (slow2) slow2.classList.remove("hidden");
        }, 650);
      }

      function setDeskError(message, href) {
        setDeskLoading(true, { href: href });
        var err = $("#desk-error");
        if (!err) return;
        var msg = $("#desk-error-message");
        if (msg) msg.textContent = message || "Load failed.";
        err.classList.remove("hidden");
      }

      window.attentionDesk = window.attentionDesk || {};
      window.attentionDesk.setLoading = setDeskLoading;
      window.attentionDesk.setError = setDeskError;
      window.attentionDesk.noteNavHref = function (href) { lastNavHref = href || ""; };

      window.addEventListener("attention:desk:swap", function () {
        setDeskLoading(false);
      });
    })();
  </script>
  <script defer src="/static/desk_nav.js?v=1"></script>
  <script defer src="/static/contact_detail.js?v=1"></script>
  <script defer src="/static/agent_rail.js?v=1"></script>
  <script defer src="/static/infer_config.js?v=1"></script>
</head>
<body class="bg-gray-50 font-sans" data-tenant-slug="{{.TenantSlug}}">
  {{template "body" .}}
</body>
</html>{{end}}`

const tenantAuthTemplate = `{{define "body"}}
  <div class="min-h-screen flex items-center justify-center px-6 py-10">
    <div class="w-full max-w-md">
      <div class="flex items-center justify-center space-x-2">
        <div class="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center">
          <span class="text-white text-sm font-semibold">A</span>
        </div>
        <span class="text-xl font-semibold text-gray-900">Attention CRM</span>
      </div>
      <div class="mt-8 bg-white rounded-2xl shadow-sm border border-gray-200 p-6">
        {{.Body}}
      </div>
    </div>
  </div>
{{end}}`

const tenantAppTemplate = `{{define "body"}}
  <div id="app-container" class="min-h-screen bg-gray-50">
    <div class="max-w-7xl mx-auto px-6 py-6">
      <div class="flex flex-col lg:flex-row items-start gap-6">
        <div class="flex-1 min-w-0 w-full">
          <header id="header" class="bg-white border border-gray-200 rounded-2xl px-6 py-3">
            <div class="flex items-center justify-between">
              <div class="flex items-center space-x-8">
                <a href="{{if .TenantSlug}}/t/{{.TenantSlug}}/app{{else}}#{{end}}" class="flex items-center space-x-2">
                  <div class="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center">
                    <svg class="w-4 h-4 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                      <path d="M13 2 3 14h7l-1 8 12-14h-7l1-6z"></path>
                    </svg>
                  </div>
                  <span class="text-xl font-semibold text-gray-900">Attention CRM</span>
                </a>
                <nav class="hidden sm:flex items-center space-x-4 text-sm">
                  <a class="text-gray-600 hover:text-gray-900 font-medium" href="/t/{{.TenantSlug}}/app">Home</a>
                  <a class="text-gray-600 hover:text-gray-900 font-medium" href="/t/{{.TenantSlug}}/deals">Deals</a>
                  <a class="text-gray-600 hover:text-gray-900 font-medium" href="/t/{{.TenantSlug}}/ledger">Ledger</a>
                  <a class="text-gray-600 hover:text-gray-900 font-medium" href="/t/{{.TenantSlug}}/members">Members</a>
                  <a class="text-gray-600 hover:text-gray-900 font-medium" href="/t/{{.TenantSlug}}/export">Export</a>
                </nav>
              </div>
              <div class="flex items-center space-x-4">
                <button type="button" class="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg" aria-label="Notifications">
                  <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                    <path d="M12 22a2 2 0 0 0 2-2h-4a2 2 0 0 0 2 2zm6-6V11a6 6 0 0 0-5-5.91V4a1 1 0 1 0-2 0v1.09A6 6 0 0 0 6 11v5l-2 2v1h16v-1l-2-2z"></path>
                  </svg>
                </button>
                <div class="w-8 h-8 rounded-full bg-gray-200"></div>
              </div>
            </div>
          </header>
          {{if .OmniBar}}
            <div class="my-6">
              {{.OmniBar}}
            </div>
          {{end}}
          <div id="desk-root" data-attention-title="{{.Title}}" class="relative min-w-0">
          <div id="desk-loading-overlay" class="hidden absolute inset-0 z-20 bg-gray-50/80 backdrop-blur-[1px]">
            <div class="h-full w-full flex items-start justify-center px-4 py-10">
              <div class="w-full max-w-3xl">
                <div class="bg-white border border-gray-200 rounded-2xl shadow-sm p-6">
                  <div class="flex items-center justify-between">
                    <div class="h-4 w-44 bg-gray-200 rounded animate-pulse"></div>
                    <div class="h-4 w-20 bg-gray-200 rounded animate-pulse"></div>
                  </div>
                  <div class="mt-6 space-y-3">
                    <div class="h-3 w-full bg-gray-200 rounded animate-pulse"></div>
                    <div class="h-3 w-11/12 bg-gray-200 rounded animate-pulse"></div>
                    <div class="h-3 w-9/12 bg-gray-200 rounded animate-pulse"></div>
                  </div>
                  <div class="mt-6 grid grid-cols-3 gap-3">
                    <div class="h-20 bg-gray-100 border border-gray-200 rounded-xl animate-pulse"></div>
                    <div class="h-20 bg-gray-100 border border-gray-200 rounded-xl animate-pulse"></div>
                    <div class="h-20 bg-gray-100 border border-gray-200 rounded-xl animate-pulse"></div>
                  </div>
                  <div id="desk-slow-indicator" class="hidden mt-6 text-sm text-gray-700">
                    <div class="flex items-center gap-2">
                      <div class="w-4 h-4 border-2 border-gray-300 border-t-blue-600 rounded-full animate-spin"></div>
                      <span>Still loading…</span>
                    </div>
                  </div>
                  <div id="desk-error" class="hidden mt-6">
                    <div class="text-sm font-medium text-red-700">Couldn’t load this desk.</div>
                    <div id="desk-error-message" class="mt-1 text-sm text-gray-700">Load failed.</div>
                    <div class="mt-3">
                      <a id="desk-retry" class="text-sm font-medium text-blue-700 hover:text-blue-800 hover:underline" href="#">Retry</a>
                    </div>
                  </div>
                </div>
                <div class="mt-4 text-xs text-gray-500">Tip: the agent rail should remain responsive during desk loads.</div>
              </div>
            </div>
          </div>
          <main id="{{if .MainID}}{{.MainID}}{{else}}main-workspace{{end}}" class="{{if .MainClass}}{{.MainClass}}{{else}}min-w-0 w-full{{end}}">
            {{.Body}}
          </main>
        </div>
        </div>
        <aside id="agent-rail" class="w-full lg:w-1/3 lg:flex-none min-w-0">
          {{.Rail}}
        </aside>
      </div>
    </div>
  </div>
{{end}}`
