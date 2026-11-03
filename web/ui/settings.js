// The settings, shared by every theme: a dialog (#settings) that a button of the theme's opens, with what one sets
// rarely. How the page looks: the language, the theme, light or dark where the theme has both, the theme's own settings
// (the pixel office's pixel size), motion paused or not. The conversation: the steps between answers shown unfolded.
// Reporting a problem: what went wrong is in the error log already (the bridge writes it as it happens); one button
// gives the report file (the errors, this bridge and this page, the bridge's last log lines; no key, no token, no chat)
// to send by email or attach to an issue, and what the log holds can be looked at. And what this is: the versions,
// where the data is.
//
//   const settings = createSettings(sym, { scheme, extra, records })
//     scheme:  { get() → 'system' | 'light' | 'dark', set(value) } for a theme with light and dark (Coworkers)
//     extra:   [{ label, el, hint }] rows of the theme's own, under how the page looks (the office's pixel size)
//     records: [{ id, label, icon, open() }] the theme's logs (the office's message and activity logs): a button each,
//              which closes the settings and shows that log
//   settings.open() · settings.close() · settings.hidden · settings.el
//   settingsButton(sym, settings, { cls, icon }) → the theme's button: "Settings", marked while the error log has
//     something newer than what was seen
import { t, lang, esc, button } from '/ui/common.js'
import { langPicker, LANGS } from '/i18n.js'
import { dialog } from '/ui/a11y.js'
import { motion } from '/ui/motion.js'
import { themePicker } from '/ui/themes.js'
import { pref, setPref, STEPS_OPEN, FAULTS_SEEN } from '/ui/prefs.js'
import { onBackdrop, showError, setText } from '/ui/errors.js'

const ISSUES = 'https://github.com/feynmanzcys-bytes/symphony/issues/new'
const SHOW = 30 // entries of the error log the dialog lists (a report has them all)

const MARKUP = `
<div class="modal pop" id="settings" hidden>
  <div class="panel settings">
    <div class="top"><h2></h2><button type="button" class="close"></button></div>
    <div class="body">
      <section class="look" aria-labelledby="set-look"><h3 id="set-look"></h3>
        <div class="row"><label for="set-lang"></label><span class="ctl lang"></span></div>
        <div class="row"><label for="set-theme"></label><span class="ctl theme"></span></div>
        <div class="row scheme" role="radiogroup" aria-labelledby="set-scheme" hidden><span class="label" id="set-scheme"></span><span class="ctl seg"></span></div>
        <div class="extra"></div>
        <div class="row check"><input type="checkbox" id="set-still"><label for="set-still"></label><small class="hint" id="set-still-hint"></small></div>
      </section>
      <section class="talk" aria-labelledby="set-talk"><h3 id="set-talk"></h3>
        <div class="row check"><input type="checkbox" id="set-steps"><label for="set-steps"></label><small class="hint" id="set-steps-hint"></small></div>
      </section>
      <section class="records" aria-labelledby="set-records" hidden><h3 id="set-records"></h3><div class="acts"></div></section>
      <section class="errlog" aria-labelledby="set-errlog"><h3 id="set-errlog"></h3>
        <p class="what"></p>
        <div class="acts"></div>
        <p class="saved" role="status"></p>
        <p class="state"></p>
        <details class="recorded" hidden><summary></summary><div class="entries"></div><p class="more" hidden></p></details>
      </section>
      <section class="about" aria-labelledby="set-about"><h3 id="set-about"></h3>
        <dl></dl>
      </section>
    </div>
  </div>
</div>`

export function createSettings(sym, { scheme = null, extra = [], records = [] } = {}) {
  const tpl = document.createElement('template')
  tpl.innerHTML = MARKUP.trim()
  const el = tpl.content.firstElementChild
  document.body.appendChild(el)
  const $ = sel => el.querySelector(sel)
  $('h2').textContent = t('settings.title')
  dialog(el, $('h2'))
  $('.close').onclick = () => settings.close()
  onBackdrop(el, () => settings.close())
  // Esc closes it, and only it (a window under it stays open)
  el.addEventListener('keydown', ev => { if (ev.key === 'Escape' && !ev.defaultPrevented) { ev.preventDefault(); ev.stopPropagation(); settings.close() } })

  // ---------- how the page looks ----------
  $('#set-look').textContent = t('settings.look')
  $('label[for=set-lang]').textContent = t('common.language')
  const langSel = langPicker()
  langSel.id = 'set-lang'
  $('.ctl.lang').append(langSel)
  $('label[for=set-theme]').textContent = t('common.theme')
  themePicker(sym).then(sel => { sel.id = 'set-theme'; $('.ctl.theme').append(sel) })
  if (scheme) {
    const box = $('.scheme')
    box.hidden = false
    box.querySelector('.label').textContent = t('settings.scheme')
    const seg = box.querySelector('.seg')
    for (const v of ['system', 'light', 'dark']) {
      const id = 'set-scheme-' + v
      seg.insertAdjacentHTML('beforeend', `<input type="radio" name="set-scheme" id="${id}" value="${v}"><label for="${id}">${esc(t('settings.scheme.' + v))}</label>`)
    }
    const mark = () => { for (const r of seg.querySelectorAll('input')) r.checked = r.value === scheme.get() }
    seg.addEventListener('change', ev => { scheme.set(ev.target.value); mark() })
    mark()
  }
  for (const x of extra) {
    const row = $('.extra').appendChild(document.createElement('div'))
    row.className = 'row'
    const label = row.appendChild(document.createElement('span'))
    label.className = 'label'
    label.id = 'set-x-' + Math.random().toString(36).slice(2, 8)
    label.textContent = x.label
    x.el.setAttribute('aria-labelledby', label.id)
    row.append(x.el)
    if (x.hint) { const h = row.appendChild(document.createElement('small')); h.className = 'hint'; h.textContent = x.hint }
  }
  const still = $('#set-still')
  $('label[for=set-still]').textContent = t('motion.pause')
  still.setAttribute('aria-describedby', 'set-still-hint')
  const drawStill = () => {
    still.checked = motion.still
    still.disabled = motion.system
    $('#set-still-hint').textContent = t(motion.system ? 'motion.system' : 'settings.motionHint')
  }
  still.onchange = () => { if (!motion.system) motion.set(still.checked) }
  motion.on(drawStill)
  drawStill()

  // ---------- the conversation ----------
  $('#set-talk').textContent = t('settings.talk')
  const steps = $('#set-steps')
  $('label[for=set-steps]').textContent = t('settings.stepsOpen')
  $('#set-steps-hint').textContent = t('settings.stepsHint')
  steps.setAttribute('aria-describedby', 'set-steps-hint')
  steps.checked = pref(STEPS_OPEN) === 'open'
  steps.onchange = () => setPref(STEPS_OPEN, steps.checked ? 'open' : null)

  // ---------- the theme's logs ----------
  // Each shows beside the page, not in here: the settings close, and the focus goes to the log (its heading, so its name
  // is read), which the theme does in open()
  if (records.length) {
    $('.records').hidden = false
    $('#set-records').textContent = t('settings.records')
    for (const r of records) {
      const b = $('.records .acts').appendChild(document.createElement('button'))
      b.type = 'button'
      b.className = 'px'
      b.id = r.id
      if (r.icon) b.append(r.icon)
      b.append(r.label)
      b.onclick = () => { settings.close(); r.open() }
    }
  }

  // ---------- reporting a problem ----------
  $('#set-errlog').textContent = t('errlog.title')
  $('.errlog .what').textContent = t('errlog.what')
  const acts = $('.errlog .acts'), saved = $('.errlog .saved'), recorded = $('.errlog .recorded')
  const state = $('.errlog .state'), entries = $('.errlog .entries')
  let list = [], diag = null, loading = null
  // the one thing to do: take the report file (the log is fetched first, so it has everything up to now)
  const give = button(t('errlog.report'), 'primary report', async () => {
    give.disabled = true
    await load()
    const name = save()
    give.disabled = false
    saved.textContent = t('errlog.saved', { file: name })
  })
  const clear = button(t('errlog.clear'), 'clear', async () => {
    if (!confirm(t('errlog.clearConfirm'))) return
    const r = await sym.clearErrors()
    if (!r.ok) return showError(t('errlog.clearFailed', { reason: r.data?.error || r.status }), { report: false })
    list = []
    draw()
  })
  // where to send it: GitHub's issues (or an email, as the person likes)
  const issues = document.createElement('a')
  issues.className = 'issues'
  issues.href = ISSUES
  issues.target = '_blank'
  issues.rel = 'noopener noreferrer'
  issues.textContent = t('settings.issues')
  for (const b of [give, clear]) b.type = 'button'
  acts.append(give, issues, clear)
  // fetched as the dialog opens and whenever the log keeps something new while it shows
  async function load() {
    if (loading) return loading
    loading = (async () => {
      const [e, d] = await Promise.all([sym.errors(), sym.diagnostics()])
      if (e.ok) list = e.data?.errors ?? []
      if (d.ok) diag = d.data
      loading = null
      draw(e.ok ? '' : t('errlog.loadFailed', { reason: e.data?.error || e.status }))
    })()
    return loading
  }
  const timeFmt = new Intl.DateTimeFormat(lang, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })
  const when = s => { const d = new Date(s); return isNaN(d) ? '' : timeFmt.format(d) }
  const source = s => t('errlog.source.' + s) === 'errlog.source.' + s ? s : t('errlog.source.' + s)
  function draw(failure = '') {
    const newest = [...list].reverse()
    state.textContent = failure || (list.length ? t('errlog.count', { n: list.length }) : t('errlog.empty'))
    state.classList.toggle('err', !!failure)
    recorded.hidden = !list.length
    recorded.querySelector('summary').textContent = t('errlog.show')
    clear.disabled = !list.length
    drawEntries(newest.slice(0, SHOW))
    const more = $('.errlog .more')
    more.hidden = newest.length <= SHOW
    setText(more, more.hidden ? '' : t('errlog.more', { n: newest.length - SHOW }))
    drawAbout()
  }
  // the newest SHOW, each a line that unfolds into its whole text; kept in place and changed only where they changed, so
  // an unfolded one, a selection in it or the focus on it stays when a new one comes in
  function drawEntries(shown) {
    const keep = new Set(shown.map(f => f.id))
    for (const d of [...entries.children]) if (!keep.has(d.dataset.id)) d.remove()
    let prev = null
    for (const f of shown) {
      let d = [...entries.children].find(x => x.dataset.id === f.id)
      if (!d) {
        d = document.createElement('details')
        d.className = 'entry'
        d.dataset.id = f.id
        d.innerHTML = `<summary><time></time><span class="src"></span><span class="txt"></span><span class="n"></span></summary><pre></pre>`
      }
      const time = d.querySelector('time'), n = d.querySelector('.n'), first = String(f.text).split('\n')[0]
      if (time.dateTime !== f.last) { time.dateTime = f.last; setText(time, when(f.last)) }
      setText(d.querySelector('.src'), source(f.source))
      setText(d.querySelector('.txt'), [f.name, f.task].filter(Boolean).join(' · ') + (f.name || f.task ? t('common.colon') : '') + first)
      setText(n, f.count > 1 ? t('errlog.times', { n: f.count }) : '')
      n.hidden = f.count <= 1
      setText(d.querySelector('pre'), entryText(f))
      if ((prev ? prev.nextElementSibling : entries.firstElementChild) !== d) { if (prev) prev.after(d); else entries.prepend(d) }
      prev = d
    }
  }
  // the log counts as seen once its section is on screen in the open dialog (not on opening it: the section may be
  // below, out of sight): measured as the dialog opens, as it scrolls, and when something new comes in (an
  // IntersectionObserver isn't told when the dialog hides, so it can't say the section came back into sight)
  const body = $('.body'), logHead = $('#set-errlog')
  function inSight() {
    if (el.hidden) return false
    const b = body.getBoundingClientRect(), h = logHead.getBoundingClientRect()
    return h.height > 0 && h.bottom > b.top && h.top < b.bottom
  }
  function seen() {
    if (sym.state.faults?.last) setPref(FAULTS_SEEN, sym.state.faults.last)
    mark()
  }
  body.addEventListener('scroll', () => { if (inSight() && fresh()) seen() }, { passive: true })
  sym.on('fault', () => {
    if (!el.hidden) { load(); if (inSight()) seen() }
    mark()
  })

  // the report: plain text, to send as it is
  function report() {
    const d = diag ?? {}, lines = []
    const up = d.started ? Math.round((Date.parse(d.now) - Date.parse(d.started)) / 60000) : null
    const counted = m => Object.entries(m || {}).map(([k, n]) => `${k} ×${n}`).join(', ') || '—'
    lines.push('symphony — error report', `made: ${new Date().toISOString()} (${Intl.DateTimeFormat().resolvedOptions().timeZone})`)
    lines.push(`bridge: ${d.version || '?'} · ${d.os || '?'}/${d.arch || '?'} · ${d.go || '?'} · protocol ${d.protocol ?? '?'}` +
      (up !== null ? ` · up ${Math.floor(up / 60)}h ${up % 60}m` : '') + (d.lang ? ` · log language ${d.lang}` : ''))
    if (d.dataDir) lines.push(`data folder: ${d.dataDir}`)
    if (d.launch) {
      const m = d.launch.mod
      lines.push(`starting sessions from the page: ${d.launch.enabled ? 'on' : 'off'}` + (m ? ` · mod ${m.ok ? 'ok' : m.code || 'missing'}${m.version ? ' ' + m.version : ''}${m.id ? ' (' + m.id + ')' : ''}` : '') + (d.launch.terminal ? ` · terminal ${d.launch.terminal}` : ''))
    }
    if (d.sessions) lines.push(`sessions: ${d.sessions.online} online of ${d.sessions.known} · Claude Code ${counted(d.sessions.engines)} · mod ${counted(d.sessions.mods)} · ${counted(d.sessions.surfaces)}`)
    if (d.tasks && Object.keys(d.tasks).length) lines.push(`tasks: ${counted(d.tasks)}`)
    if (d.themes) lines.push(`themes: ${d.themes.join(', ')}`)
    const theme = document.documentElement.dataset.theme
    lines.push(`page: ${theme} · ${lang} (${LANGS.find(([l]) => l === lang)?.[1] ?? ''}) · ${innerWidth}×${innerHeight} @${devicePixelRatio}x · motion ${motion.still ? 'paused' + (motion.system ? ' (system)' : '') : 'on'}`)
    lines.push(`browser: ${navigator.userAgent}`)
    lines.push('', `errors: ${list.length}, newest first`)
    for (const f of [...list].reverse()) {
      lines.push('', `—— ${f.last} · ${f.source}${f.code ? ' · ' + f.code : ''}${f.name ? ' · ' + f.name : ''}${f.task ? ' · ' + f.task : ''}${f.count > 1 ? ` · ×${f.count} since ${f.time}` : ''}`)
      lines.push(entryText(f))
    }
    if (d.log?.length) lines.push('', `bridge log, last ${d.log.length} lines:`, ...d.log)
    return lines.join('\n') + '\n'
  }
  // the report as a file the browser saves (its downloads); its name, to say where it went
  function save() {
    const stamp = new Date().toISOString().slice(0, 19).replace(/[-:]/g, '').replace('T', '-')
    const a = document.createElement('a')
    a.href = URL.createObjectURL(new Blob([report()], { type: 'text/plain;charset=utf-8' }))
    a.download = `symphony-report-${stamp}.txt`
    document.body.appendChild(a)
    a.click()
    a.remove()
    setTimeout(() => URL.revokeObjectURL(a.href), 10000)
    return a.download
  }

  // ---------- what this is ----------
  $('#set-about').textContent = t('settings.about')
  function drawAbout() {
    const d = diag ?? {}, s = sym.state
    const engines = [...new Set(s.sessions.filter(x => sym.isOnline(x)).map(x => x.engine?.version).filter(Boolean))].join(', ')
    const mods = [...new Set(s.sessions.filter(x => sym.isOnline(x)).map(x => x.mod).filter(Boolean))].join(', ')
    const rows = [['settings.version', d.version], ['settings.system', d.os && `${d.os}/${d.arch}`], ['settings.engines', engines],
      ['settings.mods', mods], ['settings.address', location.host], ['settings.dataDir', d.dataDir]]
    $('.about dl').innerHTML = rows.filter(([, v]) => v).map(([k, v]) => `<dt>${esc(t(k))}</dt><dd>${esc(v)}</dd>`).join('')
  }

  // ---------- the button's mark ----------
  const marks = new Set() // the theme's buttons: marked while the error log has news
  function fresh() {
    const last = sym.state.faults?.last, seenAt = pref(FAULTS_SEEN)
    return !!last && (!seenAt || Date.parse(last) > Date.parse(seenAt))
  }
  function mark() {
    const news = fresh()
    for (const fn of marks) fn(news)
  }
  // (the summary comes with every snapshot too: what comes in while the log is in sight is seen)
  sym.on('snapshot', () => { if (inSight()) seen(); else mark() })

  const settings = {
    el,
    get hidden() { return el.hidden },
    open() {
      if (!el.hidden) return
      const news = fresh()
      el.hidden = false
      saved.textContent = ''
      draw()
      load()
      // opened for its news: the error log brought into sight (laid out first), where it counts as seen
      requestAnimationFrame(() => {
        if (news && !inSight()) $('.errlog').scrollIntoView({ block: 'nearest' })
        if (inSight()) seen()
      })
    },
    close() { el.hidden = true },
    toggle() { el.hidden ? settings.open() : settings.close() },
    marks,
    mark,
  }
  return settings
}

// an entry's whole text: what went wrong, then where (the request, the script's place, the page)
const entryText = f => String(f.text) + (f.detail ? '\n' + String(f.detail).split('\n').map(l => '  ' + l).join('\n') : '')

// the theme's button that opens the settings: its words, an icon of the theme's (cls 'ico gear' or an element), a mark
// while the error log has something its user hasn't seen
export function settingsButton(settings, { cls = 'px', icon = null } = {}) {
  const b = document.createElement('button')
  b.type = 'button'
  b.className = cls + ' open-settings'
  b.setAttribute('aria-haspopup', 'dialog')
  if (icon) b.append(icon)
  const words = b.appendChild(document.createElement('span'))
  words.className = 'words'
  words.textContent = t('settings.open')
  const dot = b.appendChild(document.createElement('b'))
  dot.className = 'news'
  dot.hidden = true
  dot.setAttribute('aria-hidden', 'true')
  b.title = t('settings.open')
  settings.marks.add(fresh => {
    dot.hidden = !fresh
    const name = fresh ? t('settings.openNews') : null
    if (b.getAttribute('aria-label') !== name) name ? b.setAttribute('aria-label', name) : b.removeAttribute('aria-label')
  })
  settings.mark()
  b.onclick = () => settings.toggle()
  return b
}
