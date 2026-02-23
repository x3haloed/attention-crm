package app

const omnibarClientJS = `(function(){
  var input = document.getElementById("omni-input");
  var form = document.getElementById("omni-form");
  var card = document.getElementById("omni-card");
  var icon = document.getElementById("omni-icon");
  var panel = document.getElementById("search-suggestions");
  var chipsEl = document.getElementById("omni-chips");
  if(!input || !form || !panel || !card || !icon || !chipsEl) return;

  var tenantSlug = String(card.getAttribute("data-tenant-slug") || "").trim();
  if(!tenantSlug) return;

  function tpath(p){ return "/t/" + encodeURIComponent(tenantSlug) + (p || ""); }

  var items = [];
  var selected = 0;
  var open = false;
  var pickMode = false;
  var pickPayload = null;
  var lastResult = null;
  var timer = null;
  var lastQuery = "";
  var chips = []; // [{kind:'target', id, label, href}]
  var pickRestoreContent = "";

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
    syncGuidance();
  }

  function escHtml(s){
    return (s||"").replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;").replace(/'/g,"&#39;");
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
    var m = v.match(/^\s*(note|call|email|meeting|meet|deal|contact)\s*:\s*/i);
    if(!m) return false;
    setIntentChip(m[1]);
    input.value = v.slice(m[0].length);
    return true;
  }

  function looksLikeNoteClient(q){
    var t = String(q || "").trim();
    if(!t) return false;
    var words = t.split(/\s+/).filter(Boolean);
    if(words.length >= 4) return true;
    var lower = t.toLowerCase();
    var hints = ["mentioned","discussed","said","follow up","follow-up","remind","today","tomorrow","next week","next steps"];
    for(var i=0;i<hints.length;i++){
      if(lower.indexOf(hints[i]) !== -1) return true;
    }
    return false;
  }

  var INTENTS = [
    {key:"note", label:"Note"},
    {key:"call", label:"Call"},
    {key:"email", label:"Email"},
    {key:"meeting", label:"Meeting"},
    {key:"deal", label:"Deal"},
    {key:"contact", label:"Contact"}
  ];

  function maybeAddIntentRows(out, qNow){
    var t = String(qNow || "").trim().toLowerCase();
    if(!t) return;
    var max = 2;
    for(var i=0;i<INTENTS.length;i++){
      var ik = INTENTS[i].key;
      if(ik.indexOf(t) === 0 || t.indexOf(ik) === 0){
        out.unshift({kind:"intent_mode", intent:ik, label:INTENTS[i].label});
        max--;
        if(max <= 0) break;
      }
    }
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
      href: tpath("/contacts/" + contact.id)
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
    syncGuidance();
  }

  function guidancePlaceholder(){
    var intent = getIntent();
    var target = firstTarget();
    if(pickMode) return "Pick a contact… e.g. Bob Smith";
    if(intent === "contact") return "Contact name… e.g. Bob Smith";
    if(intent === "deal"){
      if(target) return "Deal title… e.g. Renewal Q2";
      return "Deal title… then pick contact (e.g. Renewal Q2)";
    }
    if(intent === "note" || intent === "call" || intent === "email" || intent === "meeting"){
      var label = intent.charAt(0).toUpperCase() + intent.slice(1);
      if(target && target.name) return label + " for " + target.name + "… e.g. Sent proposal Friday";
      return label + "… then pick contact (e.g. Bob mentioned pricing)";
    }
    if(target && target.name) return "Type a note for " + target.name + "… e.g. Follow up Friday";
    return "Search contacts, deals, or type a note… e.g. Bob mentioned pricing";
  }

  function guidanceHelp(){
    var intent = getIntent();
    var target = firstTarget();
    var qNow = String(input.value || "").trim();
    if(pickMode) return "Type to search contacts, then press Enter to select.";
    if(intent === "contact") return "Type a name. Use Open existing if it looks like a duplicate.";
    if(intent === "deal"){
      if(target) return qNow ? "Press Enter to create this deal." : "Type a deal title, then press Enter.";
      return "Pick a contact to attach this deal to.";
    }
    if(intent === "note" || intent === "call" || intent === "email" || intent === "meeting"){
      if(target && qNow) return "Press Enter to log.";
      if(target) return "Type your message. Press Enter to log.";
      return "Pick a contact to log with (Pick contact… keeps your text).";
    }
    return "";
  }

  function syncGuidance(){
    if(!input) return;
    input.setAttribute("placeholder", guidancePlaceholder());
  }

  function render(){
    if(!open){ panel.innerHTML = ""; return; }
    var heading = (pickMode ? "Pick Contact" : "Search Results");
    var help = guidanceHelp();
    var html = '<div class="mb-3">' +
      '<div class="text-xs font-medium text-gray-500 uppercase tracking-wider">' + escHtml(heading) + '</div>' +
      (help ? '<div class="mt-1 text-xs text-gray-500">' + escHtml(help) + '</div>' : '') +
    '</div>';
    items.forEach(function(it, idx){
      if(it.kind === "intent_mode"){
        var rowClassI = 'flex items-center space-x-3 p-3 hover:bg-gray-50 rounded-lg cursor-pointer transition-colors';
        if(idx === selected){
          rowClassI = 'flex items-center space-x-3 p-3 bg-blue-50 border border-blue-200 rounded-lg cursor-pointer hover:bg-blue-100 transition-colors';
        }
        html += '<div class="'+rowClassI+'" data-idx="'+idx+'">' +
          '<div class="w-8 h-8 bg-gray-900 rounded-full flex items-center justify-center">' +
            '<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20Zm1 11H7v-2h6V7h2v6Z"/></svg>' +
          '</div>' +
          '<div class="flex-1">' +
            '<div class="text-sm font-medium text-gray-900">'+escHtml(it.label)+' mode</div>' +
            '<div class="text-xs text-gray-500">Commit a '+escHtml(it.label)+' chip</div>' +
          '</div>' +
          '<div class="text-xs text-gray-900 font-medium">Chip</div>' +
        '</div>';
        return;
      }
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
      if(it.kind === "open_existing_contact"){
        var rowClassDup = 'flex items-center space-x-3 p-3 hover:bg-gray-50 rounded-lg cursor-pointer transition-colors';
        if(idx === selected){
          rowClassDup = 'flex items-center space-x-3 p-3 bg-blue-50 border border-blue-200 rounded-lg cursor-pointer hover:bg-blue-100 transition-colors';
        }
        html += '<div class="'+rowClassDup+'" data-idx="'+idx+'">' +
          '<div class="w-8 h-8 bg-amber-500 rounded-full flex items-center justify-center">' +
            '<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 2 1 21h22L12 2Zm1 16h-2v-2h2v2Zm0-4h-2v-4h2v4Z"/></svg>' +
          '</div>' +
          '<div class="flex-1">' +
            '<div class="text-sm font-medium text-gray-900">Open existing: '+escHtml(it.name)+'</div>' +
            '<div class="text-xs text-amber-700">Possible duplicate</div>' +
          '</div>' +
          '<div class="text-xs text-blue-600 font-medium">Open</div>' +
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
      if(it.kind === "create_contact_anyway"){
        var rowClass2b = 'flex items-center space-x-3 p-3 hover:bg-gray-50 rounded-lg cursor-pointer transition-colors';
        if(idx === selected){
          rowClass2b = 'flex items-center space-x-3 p-3 bg-blue-50 border border-blue-200 rounded-lg cursor-pointer hover:bg-blue-100 transition-colors';
        }
        html += '<div class="'+rowClass2b+'" data-idx="'+idx+'">' +
          '<div class="w-8 h-8 bg-green-600 rounded-full flex items-center justify-center">' +
            '<svg class="w-3.5 h-3.5 text-white" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M19 11H13V5h-2v6H5v2h6v6h2v-6h6v-2Z"/></svg>' +
          '</div>' +
          '<div class="flex-1">' +
            '<div class="text-sm font-medium text-gray-900">Create anyway: '+escHtml(it.name)+'</div>' +
            '<div class="text-xs text-amber-700">Possible duplicate exists</div>' +
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

  function normalizeName(s){
    return String(s || "")
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, " ")
      .replace(/\s+/g, " ")
      .trim();
  }

  function findDuplicateContact(name, itemsList){
    var qn = normalizeName(name);
    if(!qn) return null;
    for(var i=0; i<(itemsList || []).length; i++){
      var it = itemsList[i];
      if(!it || it.kind !== "contact") continue;
      var cn = normalizeName(it.name);
      if(!cn) continue;
      if(cn === qn) return it;
      // Lightweight "close match": starts-with and near-equal length.
      if(cn.indexOf(qn) === 0 && Math.abs(cn.length - qn.length) <= 3) return it;
    }
    return null;
  }

  function setItems(result){
    lastResult = result || {};
    var qNow = String(input.value || "").trim();
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
    var noteLike = looksLikeNoteClient(qNow);

    if(!pickMode && !intentNow){
      if(noteLike){
        // Ensure "Note mode" is always visible for note-like input.
        items.unshift({kind:"intent_mode", intent:"note", label:"Note"});
      }else{
        maybeAddIntentRows(items, qNow);
      }
    }

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

    if(!pickMode && (intentNow === "note" || intentNow === "call" || intentNow === "email" || intentNow === "meeting") && !target){
      // If intent is locked but no target exists, the primary path is to pick a target (while keeping content).
      items.unshift({kind:"pick_target", content:qNow});
    }

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

    if(!pickMode && intentNow === "contact"){
      var cname = qNow;
      items = items.filter(function(it){
        return it.kind !== "create_contact" && it.kind !== "create_contact_anyway" && it.kind !== "open_existing_contact";
      });
      if(cname !== ""){
        var dup = findDuplicateContact(cname, items);
        if(dup && dup.id){
          items.push({
            kind: "open_existing_contact",
            id: dup.id,
            name: dup.name,
            company: dup.company || "",
            initials: dup.initials || "?"
          });
          items.push({kind:"create_contact_anyway", name: cname});
        }else{
          items.push({kind:"create_contact", name: cname});
        }
      }
    }

    if(!pickMode && !intentNow){
      // Chipless create-contact suggestion should also surface a lightweight duplicate intercept.
      // If the server suggests "Create contact", show "Open existing" + "Create anyway" when
      // an exact/close match exists in the current results list.
      var createIdx = -1;
      for(var ci = 0; ci < items.length; ci++){
        if(items[ci] && items[ci].kind === "create_contact"){ createIdx = ci; break; }
      }
      if(createIdx >= 0){
        var cname2 = String(items[createIdx].name || qNow || "").trim();
        var dup2 = findDuplicateContact(cname2, items);
        if(dup2 && dup2.id){
          items.splice(createIdx, 1,
            {kind:"open_existing_contact", id: dup2.id, name: dup2.name, company: dup2.company || "", initials: dup2.initials || "?"},
            {kind:"create_contact_anyway", name: cname2}
          );
        }
      }
    }

    // If the server provided explicit note actions, ensure they're prominent (avoid "Create contact" stealing Enter).
    if(!pickMode && !intentNow){
      var firstActionIdx = -1;
      for(var ix=0; ix<items.length; ix++){
        if(items[ix].kind === "log_interaction" || items[ix].kind === "pick_entity"){ firstActionIdx = ix; break; }
      }
      if(firstActionIdx > 0){
        var actionItems = items.splice(firstActionIdx, items.length - firstActionIdx);
        // Keep any existing intent_mode rows first, then actions, then remaining.
        var head = [];
        var rest = [];
        items.forEach(function(it){
          if(it.kind === "intent_mode"){ head.push(it); } else { rest.push(it); }
        });
        items = head.concat(actionItems, rest);
      }
    }

    selected = 0;
    setOpen(items.length > 0);
    render();
  }

  function activate(idx){
    var it = items[idx];
    if(!it) return;
    if(it.kind === "intent_mode"){
      var prev = String(input.value || "");
      setIntentChip(it.intent);
      var lower = prev.trim().toLowerCase();
      if(lower === String(it.intent || "").toLowerCase() || lower === (String(it.intent || "").toLowerCase() + " mode")){
        input.value = "";
      }else{
        // Preserve content when committing intent for note-like scenarios.
        input.value = prev;
      }
      setOpen(false);
      panel.innerHTML = "";
      items = [];
      input.focus();
      return;
    }
    if(it.kind === "contact"){
      if(pickMode && pickPayload){
        if(pickPayload.action === "create_deal"){
          var fDeal = document.createElement("form");
          fDeal.method = "POST";
          fDeal.action = tpath("/deals/quick");
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
          input.value = pickRestoreContent || "";
          setOpen(false);
          pickMode = false;
          pickPayload = null;
          pickRestoreContent = "";
          items = [];
          panel.innerHTML = "";
          input.focus();
          return;
        }
      }
      if(getIntent() !== "" || chips.length > 0){
        addTargetChip({id: it.id, name: it.name, company: it.company});
        input.value = "";
        setOpen(false);
        input.focus();
        return;
      }
      window.location.href = tpath("/contacts/" + it.id);
      return;
    }
    if(it.kind === "open_existing_contact"){
      window.location.href = tpath("/contacts/" + it.id);
      return;
    }
    if(it.kind === "create_contact"){
      createContact(String(it.name || ""));
      return;
    }
    if(it.kind === "create_contact_anyway"){
      createContact(String(it.name || ""));
      return;
    }
    if(it.kind === "log_interaction"){
      var f2 = document.createElement("form");
      f2.method = "POST";
      f2.action = tpath("/interactions/quick");
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
      pickRestoreContent = String(it.content || input.value || "").trim();
      if(!getIntent()){
        setIntentChip(String(it.interaction_type || "note"));
      }
      pickPayload = {action: "set_target"};
      // Clear input to search contacts, but preserve content so user can keep typing after picking.
      input.value = "";
      items = buildContactItems(lastResult || {});
      selected = 0;
      setOpen(items.length > 0);
      render();
      return;
    }
    if(it.kind === "pick_target"){
      pickMode = true;
      pickRestoreContent = String(it.content || input.value || "").trim();
      pickPayload = {action: "set_target"};
      input.value = "";
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
        pickRestoreContent = "";
        items = buildContactItems(lastResult || {});
        selected = 0;
        setOpen(items.length > 0);
        render();
        return;
      }
      var fd = document.createElement("form");
      fd.method = "POST";
      fd.action = tpath("/deals/quick");
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

  function createContact(name){
    var fdC = new FormData();
    fdC.append("name", String(name || ""));
    fetch(tpath("/contacts/quick"), {method:"POST", body: fdC, headers: {"Accept":"application/json"}})
      .then(function(r){ if(!r.ok) throw new Error("bad"); return r.json(); })
      .then(function(data){
        if(data && data.contact && data.contact.id){
          addTargetChip({id: data.contact.id, name: data.contact.name || name || "", company: data.contact.company || ""});
        }
        input.value = "";
        setOpen(false);
        panel.innerHTML = "";
        items = [];
        pickMode = false;
        pickPayload = null;
        input.focus();
      })
      .catch(function(){});
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
    fetch(tpath("/omni?q=" + encodeURIComponent(q)), {headers: {"Accept":"application/json"}})
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
    f.action = tpath("/interactions/quick");
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
      var itTab = items[selected];
      if(itTab && itTab.kind === "intent_mode"){
        e.preventDefault();
        var prev2 = String(input.value || "");
        setIntentChip(itTab.intent);
        var intentKey = String(itTab.intent || "").toLowerCase();
        var lower2 = prev2.trim().toLowerCase();
        if(lower2 === intentKey || lower2 === (intentKey + " mode")){
          input.value = "";
        }else if(intentKey && intentKey !== "note"){
          // If the user typed "contact John Doe" (or similar) and used Tab to commit the mode,
          // strip the leading mode token so the remaining input becomes the argument.
          var re = new RegExp("^\\s*" + intentKey.replace(/[.*+?^${}()|[\\]\\\\]/g, "\\$&") + "(\\s+|\\s*:\\s*)", "i");
          input.value = prev2.replace(re, "");
        }else{
          // Preserve note-like content when committing Note mode.
          input.value = prev2;
        }
        setOpen(false);
        pickMode = false;
        pickPayload = null;
        pickRestoreContent = "";
        items = [];
        panel.innerHTML = "";
        input.focus();
        return;
      }
      if(itTab && itTab.kind === "contact"){
        e.preventDefault();
        addTargetChip({id: itTab.id, name: itTab.name, company: itTab.company});
        if(pickMode && pickPayload && pickPayload.action === "set_target"){
          input.value = pickRestoreContent || "";
        }else{
          input.value = "";
        }
        setOpen(false);
        pickMode = false;
        pickPayload = null;
        pickRestoreContent = "";
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
        // Restore user content after canceling a pick flow.
        if(pickRestoreContent){
          input.value = pickRestoreContent;
        }
        pickRestoreContent = "";
        setItems(lastResult || {});
        return;
      }
      setOpen(false);
      return;
    }
  });

  input.addEventListener("input", function(){
    syncGuidance();
    if(maybeCommitIntentFromInput()){
      setOpen(false);
      panel.innerHTML = "";
      items = [];
    }
    if(timer) clearTimeout(timer);
    timer = setTimeout(fetchResults, 120);
  });

  renderChips();
  syncGuidance();

  // Quick-capture buttons: focus omnibar and set intent chip.
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
})();`
