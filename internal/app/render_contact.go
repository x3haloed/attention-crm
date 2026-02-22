package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"html/template"
	"strconv"
	"strings"
	"time"
)

func renderContactDetailBody(
	tenant control.Tenant,
	contact tenantdb.Contact,
	timeline []tenantdb.Interaction,
	flash string,
) template.HTML {
	var b strings.Builder
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)
	now := time.Now()

	if flash != "" {
		b.WriteString(`<div class="mb-6 bg-blue-50 border border-blue-200 rounded-lg p-3 text-sm text-blue-900">` + template.HTMLEscapeString(flash) + `</div>`)
	}

	// Identity card.
	b.WriteString(`<div id="identity-card" class="bg-white rounded-xl shadow-sm border border-gray-200 p-6 mb-6">`)
	b.WriteString(`<div class="space-y-4">`)
	b.WriteString(identityRow("email", "envelope", "email", contact.Email, "Add email"))
	b.WriteString(identityRow("phone", "phone", "tel", contact.Phone, "Add phone"))
	b.WriteString(identityRow("company", "building", "text", contact.Company, "Add company"))
	b.WriteString(`<button type="button" class="text-sm text-blue-600 hover:text-blue-700 font-medium hover:underline flex items-center space-x-2" id="add-more-btn"><span>+</span><span>Add more</span></button>`)
	b.WriteString(`<div id="optional-fields" class="hidden pt-4 border-t border-gray-100">`)
	b.WriteString(`<div class="space-y-3">`)
	b.WriteString(`<button type="button" class="w-full text-left text-sm text-blue-600 hover:text-blue-700 font-medium hover:underline flex items-center space-x-2"><span class="text-blue-600">+</span><span>Add job title</span></button>`)
	b.WriteString(`<button type="button" class="w-full text-left text-sm text-blue-600 hover:text-blue-700 font-medium hover:underline flex items-center space-x-2"><span class="text-blue-600">+</span><span>Add location</span></button>`)
	b.WriteString(`<button type="button" class="w-full text-left text-sm text-blue-600 hover:text-blue-700 font-medium hover:underline flex items-center space-x-2"><span class="text-blue-600">+</span><span>Add LinkedIn profile</span></button>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	b.WriteString(`</div></div>`)

	// Interaction composer.
	b.WriteString(`<div id="interaction-composer" class="bg-white rounded-xl shadow-sm border border-gray-200 p-6 mb-6">`)
	b.WriteString(`<form method="POST" action="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(contact.ID, 10) + `/interactions">`)
	b.WriteString(`<div class="space-y-4">`)
	b.WriteString(`<div class="flex flex-col sm:flex-row sm:items-center gap-3">`)
	b.WriteString(`<select name="type" required class="bg-gray-50 border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500 w-full sm:w-auto">`)
	b.WriteString(`<option value="note" selected>Note</option><option value="call">Call</option><option value="email">Email</option><option value="meeting">Meeting</option>`)
	b.WriteString(`</select></div>`)
	b.WriteString(`<textarea name="content" required placeholder="What happened? Discussed Q1 budget planning. Sarah mentioned they're looking to expand..." class="w-full h-20 sm:h-24 p-3 border border-gray-200 rounded-lg resize-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500 text-sm"></textarea>`)
	b.WriteString(`<div class="space-y-3"><div class="border border-gray-200 rounded-lg overflow-hidden">`)
	b.WriteString(`<label class="flex items-center space-x-2 cursor-pointer p-3 hover:bg-gray-50">`)
	b.WriteString(`<input type="checkbox" class="rounded border-gray-300 text-blue-600 focus:ring-blue-500 w-4 h-4" id="follow-up-toggle">`)
	b.WriteString(`<span class="text-sm font-medium text-gray-700">Set follow-up</span></label>`)
	b.WriteString(`<div id="follow-up-date-container" class="hidden px-3 pb-3 bg-gray-50 border-t border-gray-200">`)
	b.WriteString(`<input name="due_at" type="datetime-local" class="w-full bg-white border border-gray-200 rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-blue-500 focus:border-blue-500 mt-2">`)
	b.WriteString(`</div></div></div>`)
	b.WriteString(`<div class="flex justify-end pt-2"><button class="bg-blue-600 text-white px-6 py-2.5 rounded-lg font-medium hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 text-sm w-full sm:w-auto flex items-center justify-center space-x-2"><span>Log interaction</span></button></div>`)
	b.WriteString(`</div></form></div>`)

	// Timeline.
	b.WriteString(`<div id="timeline-section" class="bg-white rounded-xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<h3 class="text-lg font-semibold text-gray-900 mb-4">Timeline</h3>`)
	if len(timeline) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No interactions yet.</div></div>`)
		return template.HTML(b.String())
	}
	b.WriteString(`<div class="space-y-4">`)
	for _, it := range timeline {
		title, desc := splitTitleDesc(it.Content)
		itemClass := `flex items-start space-x-3 p-4 hover:bg-gray-50 rounded-lg`
		chip := ``
		action := ``
		icon := interactionIcon(it.Type, "normal")
		meta := relativeTime(it.CreatedAt, now)

		if it.CompletedAt.Valid {
			itemClass = `flex items-start space-x-3 p-4 bg-green-50 border border-green-200 rounded-lg`
			chip = `<span class="bg-green-100 text-green-800 text-xs font-medium px-2 py-1 rounded-full">Completed</span>`
			icon = interactionIcon(it.Type, "completed")
		} else if it.DueAt.Valid {
			itemClass = `flex items-start space-x-3 p-4 bg-amber-50 border border-amber-200 rounded-lg`
			chip = `<span class="bg-amber-100 text-amber-800 text-xs font-medium px-2 py-1 rounded-full">Due</span>`
			icon = interactionIcon(it.Type, "due")
			meta = "Due: " + dueDisplay(it.DueAt.String, now)
			action = `<form method="POST" action="/t/` + tenantSlugEsc + `/interactions/` + strconv.FormatInt(it.ID, 10) + `/complete" style="margin:0"><button class="text-xs text-blue-600 hover:text-blue-700 font-medium hover:underline" type="submit">Mark complete</button></form>`
		}

		b.WriteString(`<div class="` + itemClass + `">`)
		if chip != "" {
			b.WriteString(`<div class="flex items-center space-x-2">` + icon + chip + `</div>`)
		} else {
			b.WriteString(icon)
		}
		b.WriteString(`<div class="flex-1">`)
		b.WriteString(`<p class="text-sm font-medium text-gray-900">` + template.HTMLEscapeString(snippet(title, 80)) + `</p>`)
		if desc != "" {
			b.WriteString(`<p class="text-xs text-gray-600 mt-1">` + template.HTMLEscapeString(snippet(desc, 200)) + `</p>`)
		}
		b.WriteString(`<p class="text-xs text-gray-500 mt-2">` + template.HTMLEscapeString(meta) + `</p>`)
		b.WriteString(`</div>`)
		if action != "" {
			b.WriteString(action)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></div>`)

	b.WriteString(`<script>
(function(){
  var tenantSlug = "` + template.HTMLEscapeString(tenant.Slug) + `";
  var contactID = ` + strconv.FormatInt(contact.ID, 10) + `;
  var updateURL = "/t/" + tenantSlug + "/contacts/" + contactID + "/update";

  var saveIndicator = document.getElementById('save-indicator');
  function setIndicator(state){
    if(!saveIndicator) return;
    if(state === "saving"){
      saveIndicator.innerHTML = '<svg class="w-4 h-4 text-gray-400 animate-spin" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 2a10 10 0 0 1 7.07 2.93l-1.41 1.41A8 8 0 1 0 20 12h2A10 10 0 0 1 12 22 10 10 0 0 1 12 2Z"/></svg><span class="text-gray-500">Saving...</span>';
      return;
    }
    if(state === "error"){
      saveIndicator.innerHTML = '<svg class="w-4 h-4 text-red-500" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20Zm1 13h-2v2h2v-2Zm0-10h-2v8h2V5Z"/></svg><span class="text-red-600">Not saved</span>';
      return;
    }
    saveIndicator.innerHTML = '<svg class="w-4 h-4 text-green-500" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4Z"/></svg><span class="text-green-600">Saved</span>';
  }

  var timers = {};
  function scheduleSave(el){
    var field = el.getAttribute('data-field');
    if(!field) return;
    var value = el.value || "";
    setIndicator("saving");
    if(timers[field]) clearTimeout(timers[field]);
    timers[field] = setTimeout(function(){ doSave(field, value); }, 450);
  }

  function doSave(field, value){
    fetch(updateURL, {
      method: "POST",
      headers: {"Content-Type":"application/json","X-CSRF-Token": (window.attentionCsrfToken ? window.attentionCsrfToken() : "")},
      body: JSON.stringify({field: field, value: value})
    }).then(function(res){
      if(!res.ok) throw new Error("bad status");
      return res.json();
    }).then(function(){
      setIndicator("saved");
    }).catch(function(){
      setIndicator("error");
    });
  }

  var fields = document.querySelectorAll('[data-field]');
  fields.forEach(function(el){
    el.addEventListener('input', function(){ scheduleSave(el); });
    el.addEventListener('blur', function(){
      var field = el.getAttribute('data-field');
      if(timers[field]) { clearTimeout(timers[field]); timers[field]=null; }
      doSave(field, el.value || "");
    });
  });

  var addMore = document.getElementById('add-more-btn');
  var optional = document.getElementById('optional-fields');
  if (addMore && optional) addMore.addEventListener('click', function(){ optional.classList.toggle('hidden'); });

  var toggle = document.getElementById('follow-up-toggle');
  var container = document.getElementById('follow-up-date-container');
  if (toggle && container) toggle.addEventListener('change', function(){ container.classList.toggle('hidden', !toggle.checked); });
})();
</script>`)

	return template.HTML(b.String())
}

func renderContactHeader(tenant control.Tenant, contact tenantdb.Contact) template.HTML {
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)
	name := template.HTMLEscapeString(contact.Name)
	return template.HTML(`
<header id="header" class="bg-white border-b border-gray-200 px-4 py-4 lg:px-6">
  <div class="flex items-center justify-between max-w-4xl mx-auto">
    <div class="flex items-center space-x-4">
      <a href="/t/` + tenantSlugEsc + `/app" class="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg" aria-label="Back">
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M14.7 6.3 13.3 4.9 6.2 12l7.1 7.1 1.4-1.4L9 12l5.7-5.7Z"/></svg>
      </a>
      <div class="flex items-center space-x-3">
        <input type="text" value="` + name + `" class="text-xl lg:text-2xl font-semibold text-gray-900 bg-transparent border-none outline-none hover:bg-gray-50 focus:bg-white focus:ring-2 focus:ring-blue-500 rounded-lg px-2 py-1" id="contact-name" data-field="name" autocomplete="off" />
        <span class="text-xs text-gray-400 font-medium flex items-center space-x-1 transition-all duration-300" id="save-indicator">
          <svg class="w-4 h-4 text-green-500" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4Z"/></svg>
          <span class="text-green-600">Saved</span>
        </span>
      </div>
    </div>
    <div class="flex items-center space-x-2">
      <button type="button" class="p-2 text-gray-500 hover:text-gray-700 hover:bg-gray-100 rounded-lg" aria-label="Menu">
        <svg class="w-5 h-5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 7a2 2 0 1 0 0-4 2 2 0 0 0 0 4Zm0 7a2 2 0 1 0 0-4 2 2 0 0 0 0 4Zm0 7a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z"/></svg>
      </button>
    </div>
  </div>
</header>`)
}

func identityRow(field, icon, inputType, value, placeholder string) string {
	var iconPath string
	switch icon {
	case "envelope":
		iconPath = "M20 4H4a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2Zm0 4-8 5L4 8V6l8 5 8-5v2Z"
	case "phone":
		iconPath = "M6.62 10.79a15.053 15.053 0 0 0 6.59 6.59l2.2-2.2a1 1 0 0 1 1.01-.24c1.12.37 2.33.57 3.58.57a1 1 0 0 1 1 1V20a1 1 0 0 1-1 1C10.61 21 3 13.39 3 4a1 1 0 0 1 1-1h3.5a1 1 0 0 1 1 1c0 1.25.2 2.46.57 3.59a1 1 0 0 1-.24 1.01l-2.21 2.19Z"
	default: // building
		iconPath = "M4 22V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v18h-2v-2H6v2H4Zm2-4h9V4H6v14Zm2-9h2v2H8V9Zm0 4h2v2H8v-2Zm4-4h2v2h-2V9Zm0 4h2v2h-2v-2Z"
	}
	return `<div class="flex items-center space-x-3 group cursor-text">
  <svg class="w-4 h-4 text-gray-400" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="` + iconPath + `"></path></svg>
  <input type="` + template.HTMLEscapeString(inputType) + `" value="` + template.HTMLEscapeString(value) + `" class="flex-1 text-gray-900 bg-transparent border-none outline-none hover:bg-gray-50 focus:bg-white focus:ring-2 focus:ring-blue-500 rounded-lg px-2 py-1 cursor-text" placeholder="` + template.HTMLEscapeString(placeholder) + `" data-field="` + template.HTMLEscapeString(field) + `" autocomplete="off" />
  <svg class="w-3 h-3 text-gray-400 opacity-0 group-hover:opacity-100 transition-opacity" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25Zm18-11.5a1 1 0 0 0 0-1.41l-1.34-1.34a1 1 0 0 0-1.41 0l-1.13 1.13 3.75 3.75 1.13-1.13Z"/></svg>
</div>`
}
