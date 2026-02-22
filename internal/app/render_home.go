package app

import (
	"attention-crm/internal/control"
	"attention-crm/internal/tenantdb"
	"html/template"
	"strconv"
	"strings"
	"time"
)

func renderTenantAppBody(
	tenant control.Tenant,
	userID int64,
	state appViewState,
	contacts []tenantdb.Contact,
	needsAttention []tenantdb.Interaction,
	needsDeals []tenantdb.Deal,
	recent []tenantdb.Interaction,
) template.HTML {
	var b strings.Builder
	now := time.Now()
	tenantSlugEsc := template.HTMLEscapeString(tenant.Slug)

	if state.Flash != "" {
		b.WriteString(`<div class="mb-6 bg-blue-50 border border-blue-200 rounded-lg p-3 text-sm text-blue-900">` + template.HTMLEscapeString(state.Flash) + `</div>`)
	}

	// Universal action surface.
	b.WriteString(`<div id="universal-action-surface" class="mb-8"><div class="relative"><div id="omni-card" class="relative bg-white rounded-2xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<form id="omni-form" method="POST" action="/t/` + tenantSlugEsc + `/universal"><div class="flex items-center space-x-4">`)
	b.WriteString(`<svg class="w-5 h-5 text-gray-400" viewBox="0 0 512 512" fill="currentColor" aria-hidden="true" id="omni-icon"><path fill="currentColor" d="M416 208c0 45.9-14.9 88.3-40 122.7L502.6 457.4c12.5 12.5 12.5 32.8 0 45.3s-32.8 12.5-45.3 0L330.7 376c-34.4 25.2-76.8 40-122.7 40C93.1 416 0 322.9 0 208S93.1 0 208 0S416 93.1 416 208zM208 352a144 144 0 1 0 0-288 144 144 0 1 0 0 288z"></path></svg>`)
	b.WriteString(`<div id="omni-input-wrap" class="flex-1 flex flex-wrap items-center gap-2">`)
	b.WriteString(`<div id="omni-chips" class="flex flex-wrap items-center gap-2"></div>`)
	b.WriteString(`<input id="omni-input" name="q" type="text" value="` + template.HTMLEscapeString(state.UniversalText) + `" placeholder="Search contacts, deals, or add a quick note..." class="min-w-[12rem] flex-1 text-lg bg-transparent border-none outline-none placeholder-gray-400 text-gray-900" autocomplete="off" spellcheck="false">`)
	b.WriteString(`</div>`)
	b.WriteString(`</div></form>`)

	b.WriteString(`<div id="search-suggestions" class="mt-4 space-y-1 border-t border-gray-100 pt-4 hidden"></div>`)
	b.WriteString(`</div></div></div>`)

	// Client-side omnibar palette.
	b.WriteString(`<script>
(function(){
  var tenantSlug = "` + tenantSlugEsc + `";
  var input = document.getElementById("omni-input");
  var form = document.getElementById("omni-form");
  var card = document.getElementById("omni-card");
  var icon = document.getElementById("omni-icon");
  var panel = document.getElementById("search-suggestions");
  var chipsEl = document.getElementById("omni-chips");
  if(!input || !form || !panel || !card || !icon || !chipsEl) return;

  var items = [];
  var selected = 0;
  var open = false;
  var pickMode = false;
  var pickPayload = null;
  var lastResult = null;
  var timer = null;
  var lastQuery = "";
  var chips = []; // [{kind:'target', id, label, href}]

  function setOpen(isOpen){
    open = isOpen;
    panel.classList.toggle("hidden", !isOpen);
    card.classList.toggle("shadow-lg", isOpen);
    card.classList.toggle("border-2", isOpen);
    card.classList.toggle("border-blue-500", isOpen);
    card.classList.toggle("ring-2", isOpen);
    card.classList.toggle("ring-blue-500", isOpen);
    card.classList.toggle("ring-opacity-20", isOpen);
    icon.classList.toggle("text-gray-400", !isOpen);
    icon.classList.toggle("text-blue-600", isOpen);
  }

  function escHtml(s){
    return (s||"").replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/\"/g,"&quot;").replace(/'/g,"&#39;");
  }

  function hasTargetChip(id){
    for(var i=0;i<chips.length;i++){
      if(chips[i].kind === "target" && String(chips[i].id) === String(id)) return true;
    }
    return false;
  }

  function getIntent(){
    for(var i=0;i<chips.length;i++){
      if(chips[i].kind === "intent" && chips[i].intent) return String(chips[i].intent);
    }
    return "";
  }

  function firstTargetID(){
    for(var i=0;i<chips.length;i++){
      if(chips[i].kind === "target" && chips[i].id) return String(chips[i].id);
    }
    return "";
  }

  function firstTarget(){
    for(var i=0;i<chips.length;i++){
      if(chips[i].kind === "target" && chips[i].id) return chips[i];
    }
    return null;
  }

  function setIntentChip(intent){
    intent = String(intent || "").trim().toLowerCase();
    if(intent === "meet") intent = "meeting";
    if(intent !== "note" && intent !== "call" && intent !== "email" && intent !== "meeting" && intent !== "deal" && intent !== "contact") return;

    // Remove any existing intent chip, then add as the left-most chip.
    chips = chips.filter(function(c){ return c.kind !== "intent"; });
    chips.unshift({
      kind: "intent",
      intent: intent,
      label: intent.charAt(0).toUpperCase() + intent.slice(1)
    });
    renderChips();
  }

  function maybeCommitIntentFromInput(){
    var v = String(input.value || "");
    var m = v.match(/^\\s*(note|call|email|meeting|meet|deal|contact)\\s*:\\s*/i);
    if(!m) return false;
    setIntentChip(m[1]);
    input.value = v.slice(m[0].length);
    return true;
  }

  function addTargetChip(contact){
    if(!contact || !contact.id) return;
    if(hasTargetChip(contact.id)) return;
    var label = contact.name || "Contact";
    if(contact.company) label = label + " • " + contact.company;
    chips.push({
      kind: "target",
      id: contact.id,
      name: contact.name || "",
      company: contact.company || "",
      label: label,
      href: "/t/" + tenantSlug + "/contacts/" + contact.id
    });
    renderChips();
  }

  function removeChipAt(idx){
    if(idx < 0 || idx >= chips.length) return;
    chips.splice(idx, 1);
    renderChips();
  }

  function renderChips(){
    var html = "";
    chips.forEach(function(c, idx){
      if(c.kind === "intent"){
        html += '<span class="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-gray-100 border border-gray-200 text-gray-900 text-sm font-medium">' +
          '<span>'+escHtml(c.label)+'</span>' +
          '<button type="button" class="text-gray-700 hover:text-gray-900" aria-label="Remove" data-chip-idx="'+idx+'">' +
            '<svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M18.3 5.71 12 12l6.3 6.29-1.41 1.42L10.59 13.4 4.3 19.71 2.89 18.3 9.17 12 2.89 5.71 4.3 4.29l6.29 6.3 6.3-6.3 1.41 1.42Z"/></svg>' +
          '</button>' +
        '</span>';
        return;
      }
      if(c.kind === "target"){
        html += '<span class="inline-flex items-center gap-2 px-3 py-1.5 rounded-full bg-blue-50 border border-blue-200 text-blue-900 text-sm">' +
          '<a class="hover:underline" href="'+escHtml(c.href)+'">'+escHtml(c.label)+'</a>' +
          '<button type="button" class="text-blue-700 hover:text-blue-900" aria-label="Remove" data-chip-idx="'+idx+'">' +
            '<svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M18.3 5.71 12 12l6.3 6.29-1.41 1.42L10.59 13.4 4.3 19.71 2.89 18.3 9.17 12 2.89 5.71 4.3 4.29l6.29 6.3 6.3-6.3 1.41 1.42Z"/></svg>' +
          '</button>' +
        '</span>';
      }
    });
    chipsEl.innerHTML = html;
  }

  function render(){
    if(!open){ panel.innerHTML = ""; return; }
    var html = '<div class="text-xs font-medium text-gray-500 uppercase tracking-wider mb-3">' + (pickMode ? 'Pick Contact' : 'Search Results') + '</div>';
    items.forEach(function(it, idx){
      if(it.kind === "contact"){
        var rowClass = 'flex items-center space-x-3 p-3 hover:bg-gray-50 rounded-lg cursor-pointer transition-colors';
        if(idx === selected){
          rowClass = 'flex items-center space-x-3 p-3 bg-blue-50 border border-blue-200 rounded-lg cursor-pointer hover:bg-blue-100 transition-colors';
        }
        html += '<div class="'+rowClass+'" data-idx="'+idx+'">' +
          '<div class="w-8 h-8 bg-blue-600 rounded-full flex items-center justify-center"><span class="text-white text-xs font-semibold">'+escHtml(it.initials)+'</span></div>' +
          '<div class="flex-1">' +
            '<div class="flex items-center space-x-2">' +
              '<span class="text-sm font-medium text-gray-900">'+escHtml(it.name)+'</span>' +
              (it.company ? '<span class="text-xs text-gray-500">•</span><span class="text-xs text-gray-600">'+escHtml(it.company)+'</span>' : '') +
            '</div>' +
            (it.subline ? '<div class="text-xs text-gray-500">'+escHtml(it.subline)+'</div>' : '') +
          '</div>' +
          '<div class="text-xs text-blue-600 font-medium">'+(pickMode ? 'Select' : 'Open')+'</div>' +
        '</div>';
        return;
      }
      if(it.kind === "create_contact"){
        var rowClass2 = 'flex items-center space-x-3 p-3 hover:bg-gray-50 rounded-lg cursor-pointer transition-colors';
        if(idx === selected){
          rowClass2 = 'flex items-center space-x-3 p-3 bg-blue-50 border border-blue-200 rounded-lg cursor-pointer hover:bg-blue-100 transition-colors';
        }
        html += '<div class="'+rowClass2+'" data-idx="'+idx+'">' +
          '<div class="w-8 h-8 bg-green-600 rounded-full flex items-center justify-center">' +
            '<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M19 11H13V5h-2v6H5v2h6v6h2v-6h6v-2Z"/></svg>' +
          '</div>' +
          '<div class="flex-1">' +
            '<div class="text-sm font-medium text-gray-900">Create contact: '+escHtml(it.name)+'</div>' +
            '<div class="text-xs text-gray-500">Add a new contact record</div>' +
          '</div>' +
          '<div class="text-xs text-green-600 font-medium">Create</div>' +
        '</div>';
        return;
      }
      if(it.kind === "log_interaction"){
        var rowClass3 = 'flex items-center space-x-3 p-3 hover:bg-gray-50 rounded-lg cursor-pointer transition-colors';
        if(idx === selected){
          rowClass3 = 'flex items-center space-x-3 p-3 bg-blue-50 border border-blue-200 rounded-lg cursor-pointer hover:bg-blue-100 transition-colors';
        }
        var label = 'Log ' + (it.interaction_type || 'note') + ' with ' + (it.contact_name || '');
        var sub = (it.content || '').trim();
        if(sub.length > 120) sub = sub.slice(0, 117) + '...';
        html += '<div class="'+rowClass3+'" data-idx="'+idx+'">' +
          '<div class="w-8 h-8 bg-purple-600 rounded-full flex items-center justify-center">' +
            '<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M6 2h9l5 5v15a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Zm8 1.5V8h4.5L14 3.5Z"/></svg>' +
          '</div>' +
          '<div class="flex-1">' +
            '<div class="text-sm font-medium text-gray-900">'+escHtml(label)+'</div>' +
            '<div class="text-xs text-gray-500">'+escHtml(sub)+'</div>' +
            (it.due_at ? '<div class="mt-1 text-xs text-blue-700">Due: '+escHtml(it.due_at)+'</div>' : '') +
          '</div>' +
          '<div class="text-xs text-purple-700 font-medium">Log</div>' +
        '</div>';
        return;
      }
      if(it.kind === "pick_entity"){
        var rowClass4 = 'flex items-center space-x-3 p-3 hover:bg-gray-50 rounded-lg cursor-pointer transition-colors';
        if(idx === selected){
          rowClass4 = 'flex items-center space-x-3 p-3 bg-blue-50 border border-blue-200 rounded-lg cursor-pointer hover:bg-blue-100 transition-colors';
        }
        var label2 = 'Pick contact…';
        var sub2 = (it.content || '').trim();
        if(sub2.length > 120) sub2 = sub2.slice(0, 117) + '...';
        html += '<div class="'+rowClass4+'" data-idx="'+idx+'">' +
          '<div class="w-8 h-8 bg-gray-900 rounded-full flex items-center justify-center">' +
            '<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 12a5 5 0 1 0-5-5 5 5 0 0 0 5 5Zm0 2c-4.418 0-8 2.239-8 5v1h16v-1c0-2.761-3.582-5-8-5Z"/></svg>' +
          '</div>' +
          '<div class="flex-1">' +
            '<div class="text-sm font-medium text-gray-900">'+escHtml(label2)+'</div>' +
            '<div class="text-xs text-gray-500">'+escHtml(sub2)+'</div>' +
          '</div>' +
          '<div class="text-xs text-gray-900 font-medium">Choose</div>' +
        '</div>';
        return;
      }
      if(it.kind === "pick_target"){
        var rowClass4b = 'flex items-center space-x-3 p-3 hover:bg-gray-50 rounded-lg cursor-pointer transition-colors';
        if(idx === selected){
          rowClass4b = 'flex items-center space-x-3 p-3 bg-blue-50 border border-blue-200 rounded-lg cursor-pointer hover:bg-blue-100 transition-colors';
        }
        var label2b = 'Pick contact…';
        var sub2b = (it.content || '').trim();
        if(sub2b.length > 120) sub2b = sub2b.slice(0, 117) + '...';
        html += '<div class="'+rowClass4b+'" data-idx="'+idx+'">' +
          '<div class="w-8 h-8 bg-gray-900 rounded-full flex items-center justify-center">' +
            '<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 12a5 5 0 1 0-5-5 5 5 0 0 0 5 5Zm0 2c-4.418 0-8 2.239-8 5v1h16v-1c0-2.761-3.582-5-8-5Z"/></svg>' +
          '</div>' +
          '<div class="flex-1">' +
            '<div class="text-sm font-medium text-gray-900">'+escHtml(label2b)+'</div>' +
            (sub2b ? '<div class="text-xs text-gray-500">'+escHtml(sub2b)+'</div>' : '<div class="text-xs text-gray-500">Choose who this is about</div>') +
          '</div>' +
          '<div class="text-xs text-gray-900 font-medium">Choose</div>' +
        '</div>';
        return;
      }
      if(it.kind === "create_deal"){
        var rowClass5 = 'flex items-center space-x-3 p-3 hover:bg-gray-50 rounded-lg cursor-pointer transition-colors';
        if(idx === selected){
          rowClass5 = 'flex items-center space-x-3 p-3 bg-blue-50 border border-blue-200 rounded-lg cursor-pointer hover:bg-blue-100 transition-colors';
        }
        var title = (it.title || '').trim();
        if(title.length > 120) title = title.slice(0, 117) + '...';
        var hint = it.needs_pick ? 'Pick a contact to attach' : 'Creates a deal attached to selected contact';
        html += '<div class="'+rowClass5+'" data-idx="'+idx+'">' +
          '<div class="w-8 h-8 bg-purple-600 rounded-full flex items-center justify-center">' +
            '<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M20 6h-3.586l-1.707-1.707A1 1 0 0 0 14 4H10a1 1 0 0 0-.707.293L7.586 6H4a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2Zm0 12H4V8h4l2-2h4l2 2h4v10Z"/></svg>' +
          '</div>' +
          '<div class="flex-1">' +
            '<div class="text-sm font-medium text-gray-900">Create deal: '+escHtml(title || 'Untitled')+'</div>' +
            '<div class="text-xs text-gray-500">'+escHtml(hint)+'</div>' +
          '</div>' +
          '<div class="text-xs text-purple-700 font-medium">Create</div>' +
        '</div>';
        return;
      }
    });
    panel.innerHTML = html;
  }

  function buildContactItems(result){
    var out = [];
    (result.contacts || []).forEach(function(c){
      out.push({
        kind: "contact",
        id: c.id,
        name: c.name,
        company: c.company || "",
        initials: (c.name || "?").split(/\s+/).filter(Boolean).slice(0,2).map(function(p){return p[0]||"";}).join("").toUpperCase() || "?",
        subline: ""
      });
    });
    return out;
  }

  function buildItemsFromV2Rows(rows){
    var out = [];
    (rows || []).forEach(function(r){
      if(!r || !r.kind) return;
      if(r.kind === "contact"){
        out.push({
          kind: "contact",
          id: r.id,
          name: r.name,
          company: r.company || "",
          initials: (r.name || "?").split(/\s+/).filter(Boolean).slice(0,2).map(function(p){return p[0]||"";}).join("").toUpperCase() || "?",
          subline: ""
        });
        return;
      }
      if(r.kind === "log_interaction"){
        out.push({
          kind: "log_interaction",
          contact_id: r.contact_id,
          contact_name: r.contact_name || "",
          interaction_type: r.interaction_type || "note",
          content: r.content || "",
          due_at: r.due_at || ""
        });
        return;
      }
      if(r.kind === "create_contact"){
        out.push({kind:"create_contact", name: r.name});
        return;
      }
      if(r.kind === "pick_entity"){
        out.push({
          kind: "pick_entity",
          interaction_type: r.interaction_type || "note",
          content: r.content || "",
          due_at: r.due_at || ""
        });
        return;
      }
    });
    return out;
  }

  function setItems(result){
    lastResult = result || {};
    if(result && result.version === 2 && Array.isArray(result.rows)){
      items = buildItemsFromV2Rows(result.rows || []);
    }else{
      items = buildContactItems(result || {});
      (result.actions || []).forEach(function(a){
        if(a.type === "log_interaction"){
          items.push({
            kind: "log_interaction",
            contact_id: a.contact_id,
            contact_name: a.contact_name || "",
            interaction_type: a.interaction_type || "note",
            content: a.content || "",
            due_at: a.due_at || ""
          });
        }
        if(a.type === "create_contact"){
          items.push({kind:"create_contact", name: a.name});
        }
      });
    }
    if(pickMode){
      items = buildContactItems(result || {});
    }

    var intentNow = getIntent();
    var target = firstTarget();
    var qNow = String(input.value || "").trim();

    // Note/call/email/meeting: allow explicit target selection even if the query isn't note-like.
    if(!pickMode && (intentNow === "note" || intentNow === "call" || intentNow === "email" || intentNow === "meeting") && !target){
      items.push({kind:"pick_target", content: qNow});
    }

    // If a target is selected for note/call/email/meeting, prefer a single primary "log" action for that target.
    if(!pickMode && target && (intentNow === "note" || intentNow === "call" || intentNow === "email" || intentNow === "meeting")){
      items = items.filter(function(it){ return it.kind !== "log_interaction" && it.kind !== "pick_entity"; });
      if(qNow !== ""){
        items.unshift({
          kind: "log_interaction",
          contact_id: target.id,
          contact_name: target.name || "",
          interaction_type: intentNow,
          content: qNow,
          due_at: ""
        });
      }
      items.push({kind:"pick_entity", action:"log_interaction", interaction_type:intentNow, content:qNow, due_at:""});
    }

    // Local-only: deal mode rows are derived from chip state.
    if(!pickMode && intentNow === "deal"){
      var title = qNow;
      items.push({
        kind: "create_deal",
        title: title,
        needs_pick: firstTargetID() === ""
      });
      if(firstTargetID() === ""){
        items.push({kind:"pick_entity", action:"create_deal", content:title});
      }
	    }
	    // Local-only: contact mode always offers create contact from typed text.
	    if(!pickMode && intentNow === "contact"){
	      var cname = qNow;
	      // De-dupe server-provided create_contact rows; in contact mode we want exactly one.
	      items = items.filter(function(it){ return it.kind !== "create_contact"; });
	      if(cname !== ""){
	        items.push({kind:"create_contact", name: cname});
	      }
	    }
    selected = 0;
    setOpen(items.length > 0);
    render();
  }

  function activate(idx){
    var it = items[idx];
    if(!it) return;
    if(it.kind === "contact"){
      if(pickMode && pickPayload){
        if(pickPayload.action === "create_deal"){
          var fDeal = document.createElement("form");
          fDeal.method = "POST";
          fDeal.action = "/t/" + tenantSlug + "/deals/quick";
          var dc1 = document.createElement("input");
          dc1.type = "hidden";
          dc1.name = "contact_id";
          dc1.value = String(it.id || "");
          fDeal.appendChild(dc1);
          var dc2 = document.createElement("input");
          dc2.type = "hidden";
          dc2.name = "title";
          dc2.value = String(pickPayload.title || pickPayload.content || "");
          fDeal.appendChild(dc2);
          document.body.appendChild(fDeal);
          fDeal.submit();
          return;
        }
        if(pickPayload.action === "set_target"){
          addTargetChip({id: it.id, name: it.name, company: it.company});
          input.value = "";
          setOpen(false);
          pickMode = false;
          pickPayload = null;
          items = [];
          panel.innerHTML = "";
          input.focus();
          return;
        }
        var fPick = document.createElement("form");
        fPick.method = "POST";
        fPick.action = "/t/" + tenantSlug + "/interactions/quick";
        var pc1 = document.createElement("input");
        pc1.type = "hidden";
        pc1.name = "contact_id";
        pc1.value = String(it.id || "");
        fPick.appendChild(pc1);
        var pc2 = document.createElement("input");
        pc2.type = "hidden";
        pc2.name = "type";
        pc2.value = String(pickPayload.interaction_type || "note");
        fPick.appendChild(pc2);
        var pc3 = document.createElement("input");
        pc3.type = "hidden";
        pc3.name = "content";
        pc3.value = String(pickPayload.content || "");
        fPick.appendChild(pc3);
        if(pickPayload.due_at){
          var pc4 = document.createElement("input");
          pc4.type = "hidden";
          pc4.name = "due_at";
          pc4.value = String(pickPayload.due_at || "");
          fPick.appendChild(pc4);
        }
        document.body.appendChild(fPick);
        fPick.submit();
        return;
      }
      // If we already have an intent or target chips, treat Enter as "select target" rather than navigate.
      if(getIntent() !== "" || chips.length > 0){
        addTargetChip({id: it.id, name: it.name, company: it.company});
        input.value = "";
        setOpen(false);
        input.focus();
        return;
      }
      window.location.href = "/t/" + tenantSlug + "/contacts/" + it.id;
      return;
    }
    if(it.kind === "create_contact"){
      // Create via JSON so we can keep the user in the omnibar and commit a chip.
      var fdC = new FormData();
      fdC.append("name", String(it.name || ""));
      fetch("/t/" + tenantSlug + "/contacts/quick", {method:"POST", body: fdC, headers: {"Accept":"application/json"}})
        .then(function(r){ if(!r.ok) throw new Error("bad"); return r.json(); })
        .then(function(data){
          if(data && data.contact && data.contact.id){
            addTargetChip({id: data.contact.id, name: data.contact.name || it.name || "", company: data.contact.company || ""});
          }
          input.value = "";
          setOpen(false);
          panel.innerHTML = "";
          items = [];
          pickMode = false;
          pickPayload = null;
          input.focus();
        })
        .catch(function(){ /* no-op */ });
      return;
    }
    if(it.kind === "log_interaction"){
      var f2 = document.createElement("form");
      f2.method = "POST";
      f2.action = "/t/" + tenantSlug + "/interactions/quick";
      var i1 = document.createElement("input");
      i1.type = "hidden";
      i1.name = "contact_id";
      i1.value = String(it.contact_id || "");
      f2.appendChild(i1);
      var i2 = document.createElement("input");
      i2.type = "hidden";
      i2.name = "type";
      i2.value = String(it.interaction_type || "note");
      f2.appendChild(i2);
      var i3 = document.createElement("input");
      i3.type = "hidden";
      i3.name = "content";
      i3.value = String(it.content || "");
      f2.appendChild(i3);
      if(it.due_at){
        var i4 = document.createElement("input");
        i4.type = "hidden";
        i4.name = "due_at";
        i4.value = String(it.due_at || "");
        f2.appendChild(i4);
      }
      document.body.appendChild(f2);
      f2.submit();
      return;
    }
    if(it.kind === "pick_entity"){
      pickMode = true;
      pickPayload = {action: it.action || "log_interaction", interaction_type: it.interaction_type || "note", content: it.content || "", due_at: it.due_at || "", title: it.title || ""};
      items = buildContactItems(lastResult || {});
      selected = 0;
      setOpen(items.length > 0);
      render();
      return;
    }
    if(it.kind === "pick_target"){
      pickMode = true;
      pickPayload = {action: "set_target"};
      items = buildContactItems(lastResult || {});
      selected = 0;
      setOpen(items.length > 0);
      render();
      return;
    }
    if(it.kind === "create_deal"){
      var title = String(it.title || input.value || "").trim();
      if(title === "") return;
      var tid = firstTargetID();
      if(!tid){
        pickMode = true;
        pickPayload = {action:"create_deal", title:title};
        items = buildContactItems(lastResult || {});
        selected = 0;
        setOpen(items.length > 0);
        render();
        return;
      }
      var fd = document.createElement("form");
      fd.method = "POST";
      fd.action = "/t/" + tenantSlug + "/deals/quick";
      var d1 = document.createElement("input");
      d1.type = "hidden";
      d1.name = "contact_id";
      d1.value = String(tid);
      fd.appendChild(d1);
      var d2 = document.createElement("input");
      d2.type = "hidden";
      d2.name = "title";
      d2.value = String(title);
      fd.appendChild(d2);
      document.body.appendChild(fd);
      fd.submit();
      return;
    }
  }

  function fetchResults(){
    var q = (input.value || "").trim();
    lastQuery = q;
    if(q === ""){
      setOpen(false);
      panel.innerHTML = "";
      items = [];
      return;
    }
    fetch("/t/" + tenantSlug + "/omni?q=" + encodeURIComponent(q), {headers: {"Accept":"application/json"}})
      .then(function(r){ if(!r.ok) throw new Error("bad"); return r.json(); })
      .then(function(data){ if((input.value||"").trim() !== lastQuery) return; setItems(data); })
      .catch(function(){ setOpen(false); });
  }

  function submitQuickInteractionIfReady(){
    if(pickMode) return false;
    var intent = getIntent();
    if(intent !== "note" && intent !== "call" && intent !== "email" && intent !== "meeting") return false;
    var target = firstTarget();
    if(!target || !target.id) return false;
    var content = String(input.value || "").trim();
    if(content === "") return false;

    var f = document.createElement("form");
    f.method = "POST";
    f.action = "/t/" + tenantSlug + "/interactions/quick";
    var i1 = document.createElement("input");
    i1.type = "hidden";
    i1.name = "contact_id";
    i1.value = String(target.id);
    f.appendChild(i1);
    var i2 = document.createElement("input");
    i2.type = "hidden";
    i2.name = "type";
    i2.value = String(intent);
    f.appendChild(i2);
    var i3 = document.createElement("input");
    i3.type = "hidden";
    i3.name = "content";
    i3.value = String(content);
    f.appendChild(i3);
    document.body.appendChild(f);
    f.submit();
    return true;
  }

  input.addEventListener("keydown", function(e){
    if(e.key === ":"){
      var tok = String(input.value || "").trim().toLowerCase();
      if(tok === "note" || tok === "call" || tok === "email" || tok === "meeting" || tok === "meet" || tok === "deal" || tok === "contact"){
        e.preventDefault();
        setIntentChip(tok);
        input.value = "";
        setOpen(false);
        panel.innerHTML = "";
        items = [];
        input.focus();
        return;
      }
    }
    if(e.key === "Backspace" && !open){
      var q0 = (input.value || "");
      if(q0.trim() === "" && chips.length > 0){
        e.preventDefault();
        removeChipAt(chips.length - 1);
        return;
      }
    }
    if(!open){
      if(e.key === "Enter"){
        e.preventDefault();
        if(!submitQuickInteractionIfReady()){
          fetchResults();
        }
      }
      return;
    }
    if(e.key === "Tab"){
      // Commit chip-able rows with Tab (power user flow).
      var itTab = items[selected];
      if(itTab && itTab.kind === "contact"){
        e.preventDefault();
        addTargetChip({id: itTab.id, name: itTab.name, company: itTab.company});
        input.value = "";
        setOpen(false);
        pickMode = false;
        pickPayload = null;
        items = [];
        panel.innerHTML = "";
        input.focus();
        return;
      }
    }
    if(e.key === "ArrowDown"){
      e.preventDefault();
      selected = Math.min(items.length-1, selected+1);
      render();
      return;
    }
    if(e.key === "ArrowUp"){
      e.preventDefault();
      selected = Math.max(0, selected-1);
      render();
      return;
    }
    if(e.key === "Enter"){
      e.preventDefault();
      activate(selected);
      return;
    }
    if(e.key === "Escape"){
      e.preventDefault();
      if(pickMode){
        pickMode = false;
        pickPayload = null;
        setItems(lastResult || {});
        return;
      }
      setOpen(false);
      return;
    }
  });

  input.addEventListener("input", function(){
    // note: / call: / email: / meeting:
    if(maybeCommitIntentFromInput()){
      setOpen(false);
      panel.innerHTML = "";
      items = [];
    }
    if(timer) clearTimeout(timer);
    timer = setTimeout(fetchResults, 120);
  });

  // Ensure chips render at least once.
  renderChips();

  // Quick-capture buttons: focus omnibar and set intent chip.
  // Use event delegation because the buttons render *after* this script tag in the HTML.
  document.addEventListener("click", function(e){
    var btn = e.target && e.target.closest ? e.target.closest("[data-omni-intent]") : null;
    if(!btn) return;
    e.preventDefault();
    var intent = String(btn.getAttribute("data-omni-intent") || "");
    if(intent){ setIntentChip(intent); }
    input.value = "";
    setOpen(false);
    panel.innerHTML = "";
    items = [];
    pickMode = false;
    pickPayload = null;
    input.focus();
  });

  document.addEventListener("click", function(e){
    var chipBtn = e.target && e.target.closest ? e.target.closest("[data-chip-idx]") : null;
    if(chipBtn){
      e.preventDefault();
      var cidx = parseInt(chipBtn.getAttribute("data-chip-idx") || "-1", 10);
      removeChipAt(cidx);
      input.focus();
      return;
    }
    if(!card.contains(e.target)){
      setOpen(false);
      return;
    }
    var row = e.target && e.target.closest ? e.target.closest("[data-idx]") : null;
    if(!row) return;
    var idx = parseInt(row.getAttribute("data-idx") || "0", 10);
    selected = idx;
    render();
    activate(idx);
  });
})();
</script>`)

	// Possible duplicates (kept, but styled in the new system).
	if len(state.Duplicates) > 0 {
		b.WriteString(`<div class="bg-white rounded-xl shadow-sm border border-gray-200 p-6 mb-8">`)
		b.WriteString(`<div class="text-sm font-semibold text-gray-900 mb-3">Possible duplicates</div>`)
		b.WriteString(`<div class="space-y-2">`)
		for _, d := range state.Duplicates {
			label := d.Contact.Name
			if d.Contact.Company != "" {
				label += " • " + d.Contact.Company
			}
			b.WriteString(`<div class="flex items-center justify-between p-3 hover:bg-gray-50 rounded-lg">`)
			b.WriteString(`<a class="text-sm font-medium text-gray-900 hover:underline" href="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(d.Contact.ID, 10) + `">` + template.HTMLEscapeString(label) + `</a>`)
			b.WriteString(`<div class="text-xs text-gray-500">` + template.HTMLEscapeString(d.Reason) + `</div>`)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div></div>`)
	}

	// Quick capture section.
	b.WriteString(`<div id="quick-capture-section" class="mt-6"><div class="grid grid-cols-4 gap-4">`)
	b.WriteString(quickCaptureButton("New Contact", "Add person or company", "hover:border-blue-300 hover:bg-blue-50", "bg-blue-100", "group-hover:bg-blue-200", "text-blue-600", "M12 5a3 3 0 1 0 0 6 3 3 0 0 0 0-6Zm-7 14c0-3.314 2.686-6 6-6h2c3.314 0 6 2.686 6 6v1H5v-1Zm13-6v-2h2V9h-2V7h-2v2h-2v2h2v2h2Z", "contact"))
	b.WriteString(quickCaptureButton("Log Call", "Record conversation", "hover:border-green-300 hover:bg-green-50", "bg-green-100", "group-hover:bg-green-200", "text-green-600", "M6.62 10.79a15.053 15.053 0 0 0 6.59 6.59l2.2-2.2a1 1 0 0 1 1.01-.24c1.12.37 2.33.57 3.58.57a1 1 0 0 1 1 1V20a1 1 0 0 1-1 1C10.61 21 3 13.39 3 4a1 1 0 0 1 1-1h3.5a1 1 0 0 1 1 1c0 1.25.2 2.46.57 3.59a1 1 0 0 1-.24 1.01l-2.21 2.19Z", "call"))
	b.WriteString(quickCaptureButton("Quick Note", "Capture thoughts", "hover:border-yellow-300 hover:bg-yellow-50", "bg-yellow-100", "group-hover:bg-yellow-200", "text-yellow-600", "M6 2h9l5 5v15a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Zm8 1.5V8h4.5L14 3.5Z", "note"))
	b.WriteString(quickCaptureButton("New Deal", "Track opportunity", "hover:border-purple-300 hover:bg-purple-50", "bg-purple-100", "group-hover:bg-purple-200", "text-purple-600", "M20 6h-3.586l-1.707-1.707A1 1 0 0 0 14 4H10a1 1 0 0 0-.707.293L7.586 6H4a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2Zm0 12H4V8h4l2-2h4l2 2h4v10Z", "deal"))
	b.WriteString(`</div></div>`)

	b.WriteString(`<div class="grid grid-cols-12 gap-6 mt-6" id="content-grid">`)

	// Needs Attention
	b.WriteString(`<div id="needs-attention-section" class="col-span-5"><div class="bg-white rounded-xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<div class="flex items-center justify-between mb-6"><h2 class="text-lg font-semibold text-gray-900">Needs Attention</h2>`)
	b.WriteString(`<span class="bg-red-100 text-red-800 text-xs font-medium px-2 py-1 rounded-full">` + strconv.Itoa(len(needsAttention)+len(needsDeals)) + `</span></div>`)

	if len(needsAttention) == 0 && len(needsDeals) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">Nothing due right now.</div>`)
	} else {
		if len(needsAttention) > 0 {
			b.WriteString(`<div class="space-y-4">`)
			for _, it := range needsAttention {
				bgClass, iconClass, meta, actionText := attentionItemMeta(it, now)
				b.WriteString(`<div class="flex items-start space-x-3 p-3 ` + bgClass + ` rounded-lg">`)
				b.WriteString(`<svg class="w-4 h-4 ` + iconClass + ` mt-1" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 8v5l4 2-1 1.732L10 14V8h2Zm0-6a10 10 0 1 0 0 20 10 10 0 0 0 0-20Z"/></svg>`)
				b.WriteString(`<div class="flex-1">`)
				b.WriteString(`<p class="text-sm font-medium text-gray-900">Follow up with ` + template.HTMLEscapeString(it.ContactName) + `</p>`)
				b.WriteString(`<p class="text-xs mt-1 ` + iconClass + `">` + template.HTMLEscapeString(meta) + `</p>`)
				b.WriteString(`</div>`)
				b.WriteString(`<a class="text-xs font-medium hover:underline ` + iconClass + `" href="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(it.ContactID, 10) + `">` + template.HTMLEscapeString(actionText) + `</a>`)
				b.WriteString(`</div>`)
			}
			b.WriteString(`</div>`)
		}

		if len(needsDeals) > 0 {
			if len(needsAttention) > 0 {
				b.WriteString(`<div class="mt-6 pt-6 border-t border-gray-100"></div>`)
			}
			b.WriteString(`<div class="flex items-center justify-between mb-3">`)
			b.WriteString(`<div class="text-xs font-medium text-gray-500 uppercase tracking-wider">Deals</div>`)
			b.WriteString(`<a href="/t/` + tenantSlugEsc + `/deals" class="text-xs font-medium text-blue-600 hover:text-blue-700 hover:underline">View all</a>`)
			b.WriteString(`</div>`)
			b.WriteString(`<div class="space-y-3">`)
			for _, d := range needsDeals {
				title := strings.TrimSpace(d.Title)
				if title == "" {
					title = "Untitled"
				}
				meta := ""
				if strings.TrimSpace(d.NextStep) == "" {
					meta = "Next step needed"
				} else {
					meta = "Next: " + snippet(d.NextStep, 64)
				}
				if d.NextStepDueAt.Valid && d.NextStepCompleted.Valid == false {
					meta = meta + " • Due: " + dueDisplay(d.NextStepDueAt.String, now)
				}
				b.WriteString(`<a href="/t/` + tenantSlugEsc + `/deals/` + strconv.FormatInt(d.ID, 10) + `" class="block p-3 rounded-lg border border-gray-200 hover:bg-gray-50">`)
				b.WriteString(`<div class="flex items-start justify-between gap-3">`)
				b.WriteString(`<div class="min-w-0">`)
				b.WriteString(`<div class="text-sm font-medium text-gray-900 truncate">` + template.HTMLEscapeString(title) + `</div>`)
				b.WriteString(`<div class="mt-1 text-xs text-gray-600">` + template.HTMLEscapeString(meta) + `</div>`)
				b.WriteString(`</div>`)
				b.WriteString(`<div class="shrink-0">` + dealStateBadge(d.State) + `</div>`)
				b.WriteString(`</div>`)
				b.WriteString(`</a>`)
			}
			b.WriteString(`</div>`)
		}
	}
	b.WriteString(`</div></div>`)

	// Recent Interactions
	b.WriteString(`<div id="recent-interactions-section" class="col-span-7"><div class="bg-white rounded-xl shadow-sm border border-gray-200 p-6">`)
	b.WriteString(`<h2 class="text-lg font-semibold text-gray-900 mb-6">Recent Interactions</h2>`)
	if len(recent) == 0 {
		b.WriteString(`<div class="text-sm text-gray-600">No interactions yet.</div>`)
	} else {
		b.WriteString(`<div class="space-y-4">`)
		for _, it := range recent {
			title, desc := splitTitleDesc(it.Content)
			b.WriteString(`<a href="/t/` + tenantSlugEsc + `/contacts/` + strconv.FormatInt(it.ContactID, 10) + `" class="flex items-center space-x-4 p-3 hover:bg-gray-50 rounded-lg cursor-pointer">`)
			b.WriteString(`<div class="w-10 h-10 bg-blue-600 rounded-full flex items-center justify-center"><span class="text-white text-xs font-semibold">` + template.HTMLEscapeString(initials(it.ContactName)) + `</span></div>`)
			b.WriteString(`<div class="flex-1">`)
			b.WriteString(`<p class="text-sm font-medium text-gray-900">` + template.HTMLEscapeString(it.ContactName) + `</p>`)
			line := title
			if desc != "" {
				line = title + " — " + desc
			}
			b.WriteString(`<p class="text-xs text-gray-600 mt-1">` + template.HTMLEscapeString(snippet(line, 120)) + `</p>`)
			b.WriteString(`</div>`)
			b.WriteString(`<div class="text-xs text-gray-500">` + template.HTMLEscapeString(relativeTime(it.CreatedAt, now)) + `</div>`)
			b.WriteString(`</a>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div></div>`)

	b.WriteString(`</div>`)

	return template.HTML(b.String())
}
