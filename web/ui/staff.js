// The employees page (#staff), shared by every theme: every employee and what it starts with from the page, the
// bridge's account `last` (its folder, model, thinking effort, permission mode and office context), which the launch
// dialog fills in for "Start work"; one never started from the page goes by the settings until it is. It is changed
// for one employee (its row's Edit) or for the selected ones at once (the drop-downs over the list), imported from a
// list (pasted, or a file: one name per line, or the JSON that Export gives), which makes the names that aren't
// employees yet into new ones, and exported. Everything goes through POST /api/accounts/import, all of it or none;
// Delete deletes the selected ones that are off duty and in no task. A change takes effect at the employee's next start;
// names stay as they are (a name is how the sessions find each other, and its task branches' slug). It opens from the
// settings (Employees → All employees), over them; Esc closes the row being edited first, then the page.
//
//   const staff = createStaff(sym)
//   staff.open() · staff.close() · staff.hidden · staff.el
//   parseStaff(text, choices, accounts) → { entries, notes, error }   what a list to import says (exported for tests)
import { t, lang, esc, errorText, lower, dot, fill, browse, showEnd, statusText, setHTML, deferWhilePressed } from '/ui/common.js'
import { dialog } from '/ui/a11y.js'
import { onBackdrop, setText } from '/ui/errors.js'
import { folderName } from '/ui/launch.js'

const MAX = 100 // employees one import may name: the bridge's maxImport
const KEEP = '*' // the batch drop-downs' "as it is"
const FIELDS = ['dir', 'model', 'effort', 'permissionMode', 'context']

const MARKUP = `
<div class="modal pop" id="staff" hidden>
  <div class="panel staff">
    <div class="top"><h2 id="staff-title"></h2><button type="button" class="close"></button></div>
    <div class="body">
      <p class="what"></p>
      <div class="tools">
        <input type="search" id="staff-find" autocomplete="off" spellcheck="false">
        <button type="button" class="px import" aria-expanded="false" aria-controls="staff-import"></button>
        <button type="button" class="px export"></button>
      </div>
      <p class="said main" role="status"></p>
      <section class="importer" id="staff-import" aria-labelledby="staff-import-h" hidden>
        <h3 id="staff-import-h"></h3>
        <p class="what" id="staff-import-what"></p>
        <label for="staff-import-text"></label>
        <textarea id="staff-import-text" rows="6" spellcheck="false" aria-describedby="staff-import-what"></textarea>
        <div class="acts"><button type="button" class="px file"></button><input type="file" accept=".json,.txt,application/json,text/plain" tabindex="-1" hidden></div>
        <p class="sum" id="staff-sum"></p>
        <ul class="preview" aria-labelledby="staff-sum"></ul>
        <div class="acts end"><button type="button" class="px cancel"></button><button type="button" class="px primary go"></button></div>
        <p class="said" role="status"></p>
      </section>
      <div class="batch" role="group" aria-labelledby="staff-batch-n">
        <span class="n" id="staff-batch-n"></span>
        <label><span></span><select name="model"></select></label>
        <label><span></span><select name="effort"></select></label>
        <label><span></span><select name="permissionMode"></select></label>
        <label><span></span><select name="context"></select></label>
        <span class="acts"><button type="button" class="px apply"></button><button type="button" class="px delete"></button></span>
        <p class="warn" id="staff-batch-warn" hidden></p>
      </div>
      <div class="head"><input type="checkbox" id="staff-all"><label for="staff-all"></label><span class="count"></span></div>
      <ul class="list" aria-labelledby="staff-title"></ul>
    </div>
  </div>
</div>`

const EDITOR = `
<form autocomplete="off" aria-labelledby="staff-edit-h">
  <h3 id="staff-edit-h"></h3>
  <label for="staff-dir"></label>
  <div class="path"><input id="staff-dir" name="dir" spellcheck="false"><button type="button" class="px browse"></button></div>
  <div class="browser" hidden><div class="cur"></div><ul></ul><button type="button" class="px pick"></button></div>
  <div class="grid">
    <div><label for="staff-model"></label><select id="staff-model" name="model"></select></div>
    <div><label for="staff-effort"></label><select id="staff-effort" name="effort"></select></div>
    <div><label for="staff-mode"></label><select id="staff-mode" name="permissionMode"></select></div>
  </div>
  <div class="warn" id="staff-warn" hidden></div>
  <label class="check"><input type="checkbox" name="context"><span></span></label>
  <div class="acts end"><button type="button" class="px cancel"></button><button type="submit" class="px primary save"></button></div>
  <p class="said" role="status"></p>
</form>`

// a name as the bridge takes one (validName): 1 to 40 characters, none of them a control character
const goodName = name => name.length > 0 && [...name].length <= 40 && !/\p{Cc}/u.test(name)
// a folder the bridge can take: an absolute path (it checks that it's there)
const absolute = dir => /^\//.test(dir) || /^[A-Za-z]:[\\/]/.test(dir) || /^\\\\/.test(dir)

export function createStaff(sym) {
  const tpl = document.createElement('template')
  tpl.innerHTML = MARKUP.trim()
  const el = tpl.content.firstElementChild
  document.body.appendChild(el)
  const $ = sel => el.querySelector(sel)
  $('h2').textContent = t('staff.title')
  dialog(el, $('h2'))
  $('.close').onclick = () => staff.close()
  onBackdrop(el, () => staff.close())
  $('.body > .what').textContent = t('staff.what')
  const find = $('#staff-find'), list = $('.list'), all = $('#staff-all'), said = $('.said.main')
  find.placeholder = t('staff.find')
  find.setAttribute('aria-label', t('staff.find'))
  $('label[for=staff-all]').textContent = t('staff.selectAll')
  const tell = (text, err) => { said.classList.toggle('err', !!err); setText(said, text) }

  let choices = null // the bridge's settings: what one goes by until its first start, the drop-downs' choices, botHome
  const picked = new Set() // the selected employees, by id
  let editing = null // the employee whose row is being edited, by id
  const rows = new Map() // id -> its row, kept while it shows (its checkbox and button keep the focus through redraws)

  const employees = () => sym.state.accounts.filter(a => !a.advisor)
  const byName = (a, b) => a.name.localeCompare(b.name, lang)
  // what an employee starts with next: its last start's choices, or, never started from the page, the settings' (and the
  // folder it worked in by hand); a folder '' is its own
  function startsWith(acc) {
    const l = acc.last
    if (l) return { dir: l.dir || '', model: l.model || '', effort: l.effort || '', permissionMode: l.permissionMode || '', context: !!l.context, settings: false }
    const d = choices || {}
    return { dir: acc.dir || '', model: d.model || '', effort: d.effort || '', permissionMode: d.permissionMode || '', context: !!d.context, settings: true }
  }
  // its own folder: the one made for it, else the one a start would make in the default folder ('' without one)
  function ownDir(acc) {
    if (acc.home) return acc.home
    const home = choices?.botHome
    if (!home) return ''
    const sep = home.includes('\\') && !home.includes('/') ? '\\' : '/'
    return home.replace(/[\\/]+$/, '') + sep + folderName(acc.name)
  }
  // a task it is in that the office shows (running, ending, failed): it can't be deleted meanwhile
  const taskOf = acc => sym.state.tasks.find(k => k.members.some(m => m.account === acc.id)) ?? null
  function stateOf(acc) {
    const s = sym.sessionOf(acc)
    if (sym.isOnline(s)) return { state: sym.asking(s.id) ? 'waiting' : s.state, text: statusText(sym, s), busy: true }
    const task = taskOf(acc)
    if (task) return { state: task.status === 'failed' ? 'ended' : 'tool', text: t('staff.inTask', { id: task.id }), busy: true }
    return { state: 'ended', text: t('state.ended'), busy: false }
  }
  // a choice in words: a model's name, an effort, a permission mode, "Default" for Claude Code's own
  function valueText(field, v) {
    if (field === 'dir') return v || t('launch.ownDir')
    if (field === 'context') return t(v ? 'staff.on' : 'staff.off')
    if (!v) return t('option.default')
    return field === 'model' ? choices?.models?.find(o => o.value === v)?.label || v : v
  }
  // what it starts with, in a line, as the launch dialog's folded line says it
  function digest(c) {
    const parts = []
    if (c.model) parts.push(valueText('model', c.model))
    if (c.effort) parts.push(c.effort)
    if (c.permissionMode) parts.push(c.permissionMode)
    if (!c.model && !c.effort && !c.permissionMode) parts.push(t('launch.engineOwn'))
    if (c.context) parts.push(t('launch.contextOn'))
    return parts.join(' · ')
  }

  // ---------- the list ----------
  function rowFor(acc) {
    let li = rows.get(acc.id)
    if (li) return li
    li = document.createElement('li')
    li.className = 'emp'
    li.dataset.account = acc.id
    li.innerHTML = `<input type="checkbox" class="pick"><div class="info"></div><button type="button" class="px edit" aria-controls="staff-editor"></button>`
    li.querySelector('.pick').addEventListener('change', ev => { if (ev.target.checked) picked.add(acc.id); else picked.delete(acc.id); draw() })
    li.querySelector('.edit').onclick = () => editing === acc.id ? closeEditor(true) : openEditor(acc.id)
    rows.set(acc.id, li)
    return li
  }
  const attr = (node, name, value) => { if (node.getAttribute(name) !== value) node.setAttribute(name, value) }
  function drawRow(li, acc) {
    const c = startsWith(acc), st = stateOf(acc), own = ownDir(acc), on = editing === acc.id
    const pick = li.querySelector('.pick'), edit = li.querySelector('.edit')
    pick.checked = picked.has(acc.id)
    attr(pick, 'aria-label', t('staff.select', { name: acc.name }))
    setText(edit, t('staff.edit'))
    attr(edit, 'aria-label', t('staff.editOf', { name: acc.name }))
    attr(edit, 'aria-expanded', String(on))
    li.classList.toggle('open', on)
    const dir = c.dir ? `<span class="dir" title="${esc(c.dir)}">${esc(c.dir)}</span>`
      : `<span class="dir own" title="${esc(own)}"><b>${esc(t('launch.ownDir'))}</b>${own ? ' ' + esc(own) : ''}</span>`
    setHTML(li.querySelector('.info'), `<div class="who"><span class="nm">${esc(acc.name)}</span><span class="st">${dot(st.state)}${esc(st.text)}</span>` +
      (c.settings ? `<span class="tag">${esc(t('staff.fromSettings'))}</span>` : '') + `</div>` +
      `<div class="cfg">${dir}<span class="opts">${esc(digest(c))}</span></div>`)
  }
  function draw() {
    if (el.hidden) return
    const everyone = employees()
    for (const id of [...picked]) if (!everyone.some(a => a.id === id)) picked.delete(id)
    if (editing && !everyone.some(a => a.id === editing)) closeEditor(false)
    const q = lower(find.value.trim())
    const shown = everyone.filter(a => a.id === editing || !q || [a.name, startsWith(a).dir, a.home].some(v => lower(v).includes(q))).sort(byName)
    const keep = new Set(shown.map(a => a.id))
    for (const [id, li] of rows) if (!keep.has(id)) { li.remove(); rows.delete(id) }
    list.querySelector(':scope > .empty')?.remove()
    let prev = null
    const place = node => { if ((prev ? prev.nextElementSibling : list.firstElementChild) !== node) { if (prev) prev.after(node); else list.prepend(node) } prev = node }
    for (const acc of shown) {
      const li = rowFor(acc)
      drawRow(li, acc)
      place(li)
      if (editing === acc.id) place(editor)
    }
    if (!shown.length) list.insertAdjacentHTML('beforeend', `<li class="empty">${esc(t(everyone.length ? 'staff.noMatch' : 'staff.none'))}</li>`)
    setText($('.head .count'), t('staff.count', { n: everyone.length }))
    const sel = shown.filter(a => picked.has(a.id)).length
    all.checked = shown.length > 0 && sel === shown.length
    all.indeterminate = sel > 0 && sel < shown.length
    all.disabled = !shown.length
    drawBatch()
  }
  find.addEventListener('input', () => draw())
  all.addEventListener('change', () => {
    const q = lower(find.value.trim())
    const shown = employees().filter(a => !q || [a.name, startsWith(a).dir, a.home].some(v => lower(v).includes(q)))
    for (const a of shown) { if (all.checked) picked.add(a.id); else picked.delete(a.id) }
    draw()
  })

  // ---------- the selected ones: what they start with, changed together; deleted ----------
  const batch = $('.batch'), bsel = {}
  for (const name of ['model', 'effort', 'permissionMode', 'context']) {
    bsel[name] = batch.querySelector(`select[name=${name}]`)
    bsel[name].previousElementSibling.textContent = t('staff.f.' + name)
  }
  const apply = $('.batch .apply'), del = $('.batch .delete'), bwarn = $('#staff-batch-warn')
  apply.textContent = t('staff.apply')
  del.textContent = t('staff.delete')
  bwarn.textContent = t('launch.bypassWarning')
  function fillBatch() {
    for (const name of ['model', 'effort', 'permissionMode']) {
      fill(bsel[name], choices?.[{ model: 'models', effort: 'efforts', permissionMode: 'permissionModes' }[name]], '')
      bsel[name].insertAdjacentHTML('afterbegin', `<option value="${KEEP}">${esc(t('staff.keep'))}</option>`)
      bsel[name].value = KEEP
    }
    bsel.context.innerHTML = [[KEEP, 'staff.keep'], ['on', 'staff.on'], ['off', 'staff.off']].map(([v, k]) => `<option value="${v}">${esc(t(k))}</option>`).join('')
    warnBatch()
  }
  function warnBatch() {
    bwarn.hidden = bsel.permissionMode.value !== 'bypassPermissions'
    if (bwarn.hidden) bsel.permissionMode.removeAttribute('aria-describedby')
    else bsel.permissionMode.setAttribute('aria-describedby', 'staff-batch-warn')
  }
  // what the drop-downs change: the ones not "As it is" (one without its choices yet, before the bridge's settings came,
  // changes nothing)
  const change = () => Object.fromEntries(Object.entries(bsel).filter(([, s]) => s.selectedIndex >= 0 && s.value !== KEEP).map(([k, s]) => [k, k === 'context' ? s.value === 'on' : s.value]))
  function drawBatch() {
    const n = employees().filter(a => picked.has(a.id)).length
    setText($('#staff-batch-n'), n ? t('staff.selected', { n }) : t('staff.noneSelected'))
    batch.classList.toggle('none', !n)
    for (const s of Object.values(bsel)) s.disabled = !n
    apply.disabled = !n || !Object.keys(change()).length
    del.disabled = !n
  }
  for (const s of Object.values(bsel)) s.addEventListener('change', () => { warnBatch(); drawBatch() })
  apply.onclick = async () => {
    const list = employees().filter(a => picked.has(a.id)).sort(byName), c = change()
    if (!list.length || !Object.keys(c).length) return
    apply.disabled = true
    const r = await sym.importStaff(list.map(a => ({ name: a.name, ...c })))
    if (!r.ok) { drawBatch(); return tell(t('staff.applyFailed', { reason: reasonOf(r.data) }), true) }
    for (const s of Object.values(bsel)) s.value = KEEP
    warnBatch()
    drawBatch()
    tell(t('staff.applied', { n: list.length, names: list.map(a => a.name).join(', ') }))
  }
  del.onclick = async () => {
    const list = employees().filter(a => picked.has(a.id)).sort(byName)
    if (!list.length || !confirm(t('staff.deleteConfirm', { n: list.length, names: list.map(a => a.name).join(', ') }))) return
    del.disabled = true
    const gone = [], kept = []
    for (const a of list) {
      const r = await sym.deleteAccount(a.id)
      if (r.ok) { gone.push(a.name); picked.delete(a.id) } else kept.push(t('staff.notDeleted', { name: a.name, reason: errorText(r.data) }))
    }
    draw()
    tell([gone.length ? t('staff.deleted', { n: gone.length, names: gone.join(', ') }) : '', ...kept].filter(Boolean).join('\n'), kept.length > 0)
  }
  // why an import was refused: the entry and its error (its name, its place in the list), or the error itself
  const reasonOf = data => data?.params?.entry ? t('staff.entryFailed', { n: data.params.entry, name: data.params.name, reason: errorText(data) }) : errorText(data)

  // ---------- one employee's row, edited ----------
  const editor = document.createElement('li')
  editor.className = 'editing'
  editor.id = 'staff-editor'
  editor.innerHTML = EDITOR.trim()
  const form = editor.querySelector('form'), field = name => form.elements[name], esaid = form.querySelector('.said'), ewarn = form.querySelector('.warn')
  for (const [sel, key] of [['label[for=staff-dir]', 'staff.f.dir'], ['label[for=staff-model]', 'launch.model'], ['label[for=staff-effort]', 'launch.effort'],
    ['label[for=staff-mode]', 'launch.permissionMode'], ['.check span', 'launch.context'], ['.browse', 'launch.browse'], ['.pick', 'launch.pick'],
    ['.cancel', 'common.cancel'], ['.save', 'settings.save'], ['.warn', 'launch.bypassWarning']]) form.querySelector(sel).textContent = t(key)
  form.querySelector('.check').title = t('launch.contextTitle')
  const tellEditor = (text, err) => { esaid.classList.toggle('err', !!err); setText(esaid, text) }
  function warnEditor() {
    ewarn.hidden = field('permissionMode').value !== 'bypassPermissions'
    if (ewarn.hidden) field('permissionMode').removeAttribute('aria-describedby')
    else field('permissionMode').setAttribute('aria-describedby', 'staff-warn')
  }
  function openEditor(id) {
    const acc = sym.accountById(id)
    if (!acc || !choices) return
    editing = id
    const c = startsWith(acc), own = ownDir(acc)
    form.querySelector('h3').textContent = t('staff.editing', { name: acc.name })
    field('dir').value = c.dir
    field('dir').placeholder = own ? t(acc.home ? 'launch.dirOwn' : 'launch.dirAuto', { dir: own }) : t('launch.dirPlaceholder')
    fill(field('model'), choices.models, c.model)
    fill(field('effort'), choices.efforts, c.effort)
    fill(field('permissionMode'), choices.permissionModes, c.permissionMode)
    field('context').checked = c.context
    form.querySelector('.browser').hidden = true
    form.querySelector('.save').disabled = false
    tellEditor('')
    warnEditor()
    draw()
    field('dir').focus({ preventScroll: true })
    showEnd(field('dir'))
    editor.scrollIntoView({ block: 'nearest' })
  }
  function closeEditor(back) {
    const id = editing
    editing = null
    editor.remove()
    draw()
    if (back) rows.get(id)?.querySelector('.edit')?.focus()
  }
  field('permissionMode').addEventListener('change', warnEditor)
  form.addEventListener('input', () => tellEditor('')) // (what saving said is about what was there)
  form.querySelector('.cancel').onclick = () => closeEditor(true)
  form.querySelector('.browse').onclick = () => browse(sym, form.querySelector('.browser'), field('dir').value.trim(), reason => tellEditor(reason, true), choices?.botHome)
  form.querySelector('.pick').onclick = () => {
    field('dir').value = form.querySelector('.browser').dataset.path
    showEnd(field('dir'))
    form.querySelector('.browser').hidden = true
  }
  form.addEventListener('submit', async ev => {
    ev.preventDefault()
    const acc = sym.accountById(editing)
    if (!acc) return closeEditor(false)
    const dir = field('dir').value.trim()
    if (dir && !absolute(dir)) return tellEditor(t('settings.saveFailed', { reason: t('error.launch_dir_not_absolute') }), true)
    const save = form.querySelector('.save')
    save.disabled = true
    tellEditor('')
    const r = await sym.importStaff([{ name: acc.name, dir, model: field('model').value, effort: field('effort').value, permissionMode: field('permissionMode').value, context: field('context').checked }])
    save.disabled = false
    if (!r.ok) return tellEditor(t('settings.saveFailed', { reason: errorText(r.data) }), true)
    closeEditor(true)
    tell(t('staff.saved', { name: acc.name }))
  })

  // ---------- importing a list ----------
  const importer = $('.importer'), importBtn = $('.tools .import'), text = $('#staff-import-text'), fileIn = $('.importer input[type=file]')
  const go = $('.importer .go'), isaid = $('.importer .said')
  importBtn.textContent = t('staff.import')
  $('#staff-import-h').textContent = t('staff.importTitle')
  $('#staff-import-what').textContent = t('staff.importWhat', { max: MAX })
  $('label[for=staff-import-text]').textContent = t('staff.importText')
  text.placeholder = t('staff.importPlaceholder')
  $('.importer .file').textContent = t('staff.chooseFile')
  $('.importer .cancel').textContent = t('common.cancel')
  const tellImport = (msg, err) => { isaid.classList.toggle('err', !!err); setText(isaid, msg) }
  let parsed = { entries: [], items: [], error: '' }
  let sending = [] // what Import sends: the items that make or change someone (one that stays as it is isn't sent, so it
  // goes on as it was: one going by the settings keeps going by them), each with its place in the list
  function showImporter(on) {
    importer.hidden = !on
    importBtn.setAttribute('aria-expanded', String(on))
    if (on) { preview(); text.focus() }
  }
  importBtn.onclick = () => showImporter(importer.hidden)
  $('.importer .cancel').onclick = () => { showImporter(false); importBtn.focus() }
  $('.importer .file').onclick = () => fileIn.click()
  fileIn.addEventListener('change', async () => {
    const f = fileIn.files?.[0]
    fileIn.value = ''
    if (!f) return
    if (f.size > 1 << 20) return tellImport(t('staff.fileTooBig', { file: f.name }), true)
    try { text.value = await f.text() } catch (e) { return tellImport(t('staff.fileUnread', { file: f.name, reason: e?.message || String(e) }), true) }
    tellImport('')
    preview()
    text.focus()
  })
  let typing = 0
  text.addEventListener('input', () => { clearTimeout(typing); typing = setTimeout(preview, 150) })
  // what the list would do, entry by entry: a new employee, one whose choices change (from → to), one that stays as it
  // is, or what's wrong with it; Import goes once nothing is wrong and something would change
  function preview() {
    if (importer.hidden) return
    parsed = choices ? parseStaff(text.value, choices, sym.state.accounts) : { entries: [], items: [], error: '' }
    const by = new Map(employees().map(a => [a.name, a])), rows = []
    let made = 0, changed = 0, bad = parsed.error ? 1 : 0
    sending = []
    for (const it of parsed.items) {
      const e = it.entry, extra = it.notes
      if (it.bad.length) { bad++; rows.push(['bad', e?.name || t('staff.entry', { n: it.n }), it.bad.concat(extra)]); continue }
      const acc = by.get(e.name)
      if (!acc) {
        made++
        sending.push(it)
        const given = FIELDS.filter(f => f in e)
        rows.push(['new', e.name, (given.length ? given.map(f => t('staff.f.' + f) + t('common.colon') + valueText(f, e[f])) : [t('staff.fromSettings')]).concat(extra)])
        continue
      }
      const now = startsWith(acc), diff = FIELDS.filter(f => f in e && e[f] !== now[f])
      if (!diff.length) { rows.push(['same', e.name, [t('staff.same')].concat(extra)]); continue }
      changed++
      sending.push(it)
      rows.push(['change', e.name, diff.map(f => `${t('staff.f.' + f)}${t('common.colon')}${valueText(f, now[f])} → ${valueText(f, e[f])}`).concat(extra)])
    }
    const item = (k, name, what) => `<li class="${k}"><span class="k">${esc(t('staff.kind.' + k))}</span>${name ? `<b>${esc(name)}</b>` : ''}<span class="w">${esc(what.join(' · '))}</span></li>`
    setHTML($('.importer .preview'), parsed.error ? item('bad', '', [parsed.error]) : rows.map(r => item(...r)).join(''))
    const sum = $('#staff-sum')
    if (!text.value.trim()) setText(sum, t('staff.importNone'))
    else if (parsed.entries.length > MAX) { bad++; setText(sum, t('error.import_too_many', { max: MAX })) }
    else setText(sum, t('staff.summary', { made, changed, bad }))
    sum.classList.toggle('err', bad > 0)
    go.disabled = bad > 0 || !(made + changed)
    setText(go, made + changed ? t('staff.importGo', { n: made + changed }) : t('staff.importGoNone'))
  }
  go.onclick = async () => {
    const items = sending
    if (!items.length) return
    go.disabled = true
    tellImport('')
    const r = await sym.importStaff(items.map(it => it.entry))
    if (!r.ok) {
      go.disabled = false
      // (the bridge counts the entries sent; the list's own place is the one to fix)
      const data = r.data?.params?.entry ? { ...r.data, params: { ...r.data.params, entry: items[r.data.params.entry - 1]?.n ?? r.data.params.entry } } : r.data
      return tellImport(t('staff.importFailed', { reason: reasonOf(data) }), true)
    }
    text.value = ''
    showImporter(false)
    importBtn.focus()
    tell(t('staff.imported', { made: r.data?.created?.length ?? 0, changed: r.data?.updated?.length ?? 0 }))
  }

  // ---------- exporting the list ----------
  // the list as a file the browser saves: what each one starts with, as an import reads it back (one going by the
  // settings, its name alone, so it goes on going by them)
  const exportBtn = $('.tools .export')
  exportBtn.textContent = t('staff.export')
  exportBtn.onclick = () => {
    const out = employees().sort(byName).map(a => a.last ? { name: a.name, dir: a.last.dir || '', model: a.last.model || '', effort: a.last.effort || '', permissionMode: a.last.permissionMode || '', context: !!a.last.context } : { name: a.name })
    const stamp = new Date().toISOString().slice(0, 19).replace(/[-:]/g, '').replace('T', '-')
    const link = document.createElement('a')
    link.href = URL.createObjectURL(new Blob([JSON.stringify({ employees: out }, null, 2) + '\n'], { type: 'application/json' }))
    link.download = `symphony-employees-${stamp}.json`
    document.body.appendChild(link)
    link.click()
    link.remove()
    setTimeout(() => URL.revokeObjectURL(link.href), 10000)
    tell(t('staff.exported', { n: out.length, file: link.download }))
  }

  // Esc: the row being edited first, then the page (back to the settings under it)
  el.addEventListener('keydown', ev => {
    if (ev.key !== 'Escape' || ev.defaultPrevented) return
    ev.preventDefault()
    ev.stopPropagation()
    if (editing) closeEditor(true)
    else staff.close()
  })
  // kept up as the office changes, once a frame, not while a pointer is down (a row replaced under a press loses its click)
  let drawing = false
  const redraw = deferWhilePressed(() => { draw(); preview() })
  sym.on('change', () => {
    if (el.hidden || drawing) return
    drawing = true
    requestAnimationFrame(() => { drawing = false; redraw() })
  })

  const staff = {
    el,
    get hidden() { return el.hidden },
    async open() {
      if (!el.hidden) return
      tell('')
      el.hidden = false
      draw()
      const r = await sym.settings()
      if (!r.ok) return tell(t('settings.loadFailed', { reason: errorText(r.data) }), true)
      choices = r.data
      fillBatch()
      draw()
      preview()
    },
    close() {
      if (editing) closeEditor(false)
      el.hidden = true
    },
  }
  return staff
}

// What a list to import says: the entries for the bridge ({name, dir?, model?, effort?, permissionMode?, context?}, as
// the employees page sends them) and, entry by entry, what's wrong with it (bad: it stops the import) or worth saying
// (notes: what's left out), or an error when it can't be read at all. The list is JSON (an array, or {employees: [...]},
// of objects or names), or a name on each line (# starts a comment). The drop-downs' values are taken whatever their
// case ("Opus", "default"); context is true or false (yes/no, on/off, 1/0 too).
//   → { entries, items: [{ n, entry, bad: [text], notes: [text] }], error }   n: the entry's place in the list, from 1
export function parseStaff(text, choices, accounts = []) {
  const src = String(text ?? '').trim(), out = { entries: [], items: [], error: '' }
  if (!src) return out
  let list, lines = false
  if (/^[[{]/.test(src)) {
    let data
    try { data = JSON.parse(src) } catch (e) { out.error = t('staff.badJSON', { reason: e?.message || String(e) }); return out }
    list = Array.isArray(data) ? data : Array.isArray(data?.employees) ? data.employees : null
    if (!list) { out.error = t('staff.noList'); return out }
  } else {
    list = src.split(/\r?\n/).map(l => l.trim()).filter(l => l && !l.startsWith('#'))
    lines = true
  }
  const advisors = new Set(accounts.filter(a => a.advisor).map(a => a.name)), seen = new Set()
  const options = { model: choices?.models || [], effort: choices?.efforts || [], permissionMode: choices?.permissionModes || [] }
  list.forEach((item, i) => {
    const it = { n: i + 1, entry: null, bad: [], notes: [] }
    out.items.push(it)
    if (typeof item === 'string') item = { name: item }
    if (!item || typeof item !== 'object' || Array.isArray(item) || typeof item.name !== 'string') return it.bad.push(t('staff.badItem'))
    const e = it.entry = { name: item.name.trim() }
    out.entries.push(e)
    if (!goodName(e.name)) it.bad.push(t('error.account_bad_name'))
    else if (seen.has(e.name)) it.bad.push(t('error.import_duplicate'))
    else if (advisors.has(e.name)) it.bad.push(t('error.account_name_taken'))
    seen.add(e.name)
    if (lines && /[\t,;]/.test(e.name)) it.notes.push(t('staff.oneName')) // (a spreadsheet's columns: the whole line is the name)
    for (const [k, v] of Object.entries(item)) {
      if (k === 'name' || v === null || v === undefined) continue
      if (k in options) {
        const want = String(v).trim().toLowerCase(), hit = options[k].find(o => o.value.toLowerCase() === want || (o.value === '' && want === 'default'))
        if (hit) e[k] = hit.value
        else it.bad.push(t('staff.badValue', { field: t('staff.f.' + k), value: String(v) }))
      } else if (k === 'dir') {
        if (typeof v !== 'string') it.bad.push(t('staff.badValue', { field: t('staff.f.dir'), value: JSON.stringify(v) }))
        else if (v.trim() && !absolute(v.trim())) it.bad.push(t('error.launch_dir_not_absolute'))
        else e.dir = v.trim()
      } else if (k === 'context') {
        const want = String(v).trim().toLowerCase()
        if (v === true || /^(true|yes|on|1)$/.test(want)) e.context = true
        else if (v === false || /^(false|no|off|0)$/.test(want)) e.context = false
        else it.bad.push(t('staff.badValue', { field: t('staff.f.context'), value: String(v) }))
      } else it.notes.push(t('staff.unknownField', { field: k }))
    }
  })
  return out
}
