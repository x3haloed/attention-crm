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
  <script>
    window.attentionCsrfToken = function () {
      var m = document.querySelector('meta[name="attention-csrf"]');
      return m ? (m.getAttribute("content") || "") : "";
    };
  </script>
</head>
<body class="bg-gray-50 font-sans">
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
    {{if .Header}}
      {{.Header}}
	  {{else}}
	      <header id="header" class="bg-white border-b border-gray-200 px-6 py-4">
	        <div class="flex items-center justify-between max-w-7xl mx-auto">
	          <div class="flex items-center space-x-8">
	            <a href="#" class="flex items-center space-x-2">
              <div class="w-8 h-8 bg-blue-600 rounded-lg flex items-center justify-center">
                <svg class="w-4 h-4 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                  <path d="M13 2 3 14h7l-1 8 12-14h-7l1-6z"></path>
                </svg>
              </div>
              <span class="text-xl font-semibold text-gray-900">Attention CRM</span>
            </a>
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
	        {{if .OmniBar}}
	          <div class="max-w-7xl mx-auto pt-4">
	            {{.OmniBar}}
	          </div>
	        {{end}}
	      </header>
	    {{end}}
    <div id="desk-root" data-attention-title="{{.Title}}">
      <main id="{{if .MainID}}{{.MainID}}{{else}}main-workspace{{end}}" class="{{if .MainClass}}{{.MainClass}}{{else}}max-w-7xl mx-auto px-6 py-8{{end}}">
        {{.Body}}
      </main>
    </div>
  </div>
{{end}}`
