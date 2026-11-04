// The launch dialog (#launch), shared by every theme: starting a session from the page for a new employee (its name)
// or an existing one ("Start work"). Where and how it works (the folder, the model, the effort, the permission mode,
// the session, the office context) fold into Advanced settings, a line that says what the start will use: for a new
// employee what the settings say (the bridge's: Settings → Employees), for one going back to work its last choices (the
// settings' when it was never started from the page). Unfolded, each can be changed for this start, or all set back to
// the settings'; a bypassPermissions warning shows folded too. The bridge starts claude in that folder; the dialog
// follows the launch in the snapshot and closes once it's online. Without a folder the employee works in its own: the
// one made for it before, else a new one in the bridge's default folder (the settings), named after it and made a git
// repository; the folded line, the field's placeholder and the preview say which.
//
//   const launch = createLaunchDialog(sym, { options, onOnline })
//   launch.open(accountId | null)    the dialog for an existing employee, or a new one
//   launch.close()
//   launch.watch()                   on every snapshot: closes once the launch is online, shows why it failed
//   launch.el
import { t, lang, esc, errorText, base, fill, browse, showEnd } from '/ui/common.js'
import { dialog } from '/ui/a11y.js'
import { putErrorBox, showError, onBackdrop } from '/ui/errors.js'
import { lookPicker, lookOf } from '/ui/faces.js'

const MARKUP = `
<div class="modal" id="launch" hidden>
  <form class="panel" autocomplete="off">
    <h2 class="title"></h2>
    <label for="launch-name" data-i18n="launch.name"></label>
    <input id="launch-name" name="name" data-i18n-placeholder="launch.namePlaceholder" required>
    <div class="look-row"><span class="label" data-i18n="launch.look"></span><div class="look"></div></div>
    <details class="adv">
      <summary><b data-i18n="launch.advanced"></b> <span class="digest"></span></summary>
      <div class="advbody">
        <div class="from"><span class="adv-note"></span><button type="button" class="px use" data-i18n="launch.useSettings" hidden></button></div>
        <label for="launch-dir" data-i18n="launch.dir"></label>
        <div class="row"><input id="launch-dir" name="dir"><button type="button" class="px browse" data-i18n="launch.browse"></button></div>
        <div class="browser" hidden><div class="cur"></div><ul></ul><button type="button" class="px pick" data-i18n="launch.pick"></button></div>
        <div class="recent"></div>
        <div class="grid">
          <div><label for="launch-model" data-i18n="launch.model"></label><select id="launch-model" name="model"></select></div>
          <div><label for="launch-effort" data-i18n="launch.effort"></label><select id="launch-effort" name="effort"></select></div>
          <div><label for="launch-mode" data-i18n="launch.permissionMode"></label><select id="launch-mode" name="permissionMode"></select></div>
          <div><label for="launch-resume" data-i18n="launch.resume"></label><select id="launch-resume" name="resume"></select></div>
          <div class="warn" id="launch-warn" data-i18n="launch.bypassWarning" hidden></div>
        </div>
        <label class="check context"><input type="checkbox" name="context"><span data-i18n="launch.context"></span></label>
        <details class="cmd"><summary data-i18n="launch.showCommand"></summary><pre class="preview"></pre></details>
      </div>
    </details>
    <div class="warn out" data-i18n="launch.bypassWarning" hidden></div>
    <div class="status"></div>
    <div class="actions"><button type="button" class="px cancel" data-i18n="common.cancel"></button><button type="submit" class="px primary go" data-i18n="launch.go"></button></div>
  </form>
</div>`

// options: the theme's loader of /api/launch/options (it may show in its own UI why starting is off); onOnline: the
// launch came online (the theme opens the new employee's window); onOpen: the dialog opens (the theme closes its others)
export function createLaunchDialog(sym, { options = () => sym.launchOptions(true), onOnline = () => {}, onOpen = () => {} } = {}) {
  const tpl = document.createElement('template')
  tpl.innerHTML = MARKUP.trim()
  const dlg = tpl.content.firstElementChild
  document.body.appendChild(dlg)
  for (const el of dlg.querySelectorAll('[data-i18n]')) el.textContent = t(el.dataset.i18n)
  for (const el of dlg.querySelectorAll('[data-i18n-placeholder]')) el.placeholder = t(el.dataset.i18nPlaceholder)
  const form = dlg.querySelector('form')
  form.querySelector('.context').title = t('launch.contextTitle')
  dialog(dlg, form.querySelector('.title'))
  const field = name => form.elements[name]
  const adv = form.querySelector('.adv')
  // a new employee's face: what its name would pick until one is picked here (the picker follows the name typed)
  const lookRow = form.querySelector('.look-row'), look = lookPicker('launch')
  lookRow.querySelector('.look').appendChild(look)
  let lookPicked = false
  look.addEventListener('change', () => { lookPicked = true })
  field('name').addEventListener('input', () => { if (!lookPicked) look.set(lookOf(field('name').value.trim())) })
  let opts = null
  let pending = null // the launch we're waiting to see online
  let account = null // the existing employee going back to work; null for a new one
  let fromLast = false // the fields were filled in with that employee's last choices, not the settings'

  // why the page can't start sessions: the mod's problem, or no launcher at all
  const offReason = () => opts?.mod ? errorText(opts.mod) : t('error.launch_disabled')

  // the status line: what goes on, or an error in a box (a failed start is in the error log already: the bridge keeps
  // it, so the page doesn't report it again)
  function setStatus(text, err) {
    const el = form.querySelector('.status')
    el.className = 'status' + (err ? ' err' : '')
    if (err) { putErrorBox(el, text, { report: false }); el.scrollIntoView({ block: 'nearest' }) }
    else el.textContent = text
  }
  // the settings' choices (what a new employee starts with: the bridge's settings), as the fields hold them
  function settingsChoices() {
    const d = opts?.defaults || {}
    return { dir: '', model: d.model || '', effort: d.effort || '', permissionMode: d.permissionMode || '', resume: '', context: !!d.context }
  }
  function put(c) {
    field('dir').value = c.dir
    fill(field('model'), opts.models, c.model)
    fill(field('effort'), opts.efforts, c.effort)
    fill(field('permissionMode'), opts.permissionModes, c.permissionMode)
    fill(field('resume'), opts.resumes, c.resume)
    field('context').checked = !!c.context
  }
  function request() {
    const r = { dir: field('dir').value.trim(), model: field('model').value, effort: field('effort').value,
      permissionMode: field('permissionMode').value, resume: field('resume').value, lang }
    if (field('context').checked) r.context = true // the session hears who else is in the office (SYMPHONY_CONTEXT=1)
    if (account) r.account = account
    else { r.name = field('name').value.trim(); r.look = look.value }
    return r
  }
  // the folder an employee started without one gets: the one made for it before, else a new one in the bridge's default
  // folder named as the bridge names it (it adds -2, -3 when that's taken); '' when the bridge has no default folder
  function ownDir() {
    const acc = account ? sym.accountById(account) : null
    if (acc?.home) return acc.home
    const home = opts?.botHome
    if (!home) return ''
    const sep = home.includes('\\') && !home.includes('/') ? '\\' : '/'
    return home.replace(/[\\/]+$/, '') + sep + folderName(acc ? acc.name : field('name').value.trim())
  }
  // command preview: the same arguments the bridge builds (it adds the environment variables, and --plugin-dir in development mode)
  function preview() {
    const r = request()
    const q = s => /^[\w\-./=:@]+$/.test(s) ? s : `'${s.replace(/'/g, `'\\''`)}'`
    const own = ownDir(), acc = account ? sym.accountById(account) : null
    field('dir').placeholder = own ? t(acc?.home ? 'launch.dirOwn' : 'launch.dirAuto', { dir: own }) : t('launch.dirPlaceholder')
    const args = ['claude', '--name', q(field('name').value.trim() || '?')]
    if (r.model) args.push('--model', r.model)
    if (r.effort) args.push('--effort', r.effort)
    if (r.permissionMode) args.push('--permission-mode', r.permissionMode)
    if (r.resume === 'continue') args.push('--continue')
    const note = t(opts?.mod?.dev ? 'launch.previewNoteDev' : 'launch.previewNote')
    form.querySelector('.preview').textContent = `cd ${q(r.dir || own || '?')}\n${r.context ? 'SYMPHONY_CONTEXT=1 ' : ''}${args.join(' ')}\n# ${note}`
    // the folded line (the whole folder on hover), and "Use the settings" while a field differs from them
    const line = form.querySelector('.digest')
    line.textContent = digest(r, own)
    line.title = r.dir || own
    const s = settingsChoices()
    form.querySelector('.use').hidden = r.dir === s.dir && r.model === s.model && r.effort === s.effort &&
      r.permissionMode === s.permissionMode && r.resume === s.resume && !!r.context === s.context
  }
  // what the start will use, in a line: the folder's name (or its own, made as it starts), the model, the effort and
  // the permission mode picked (none: Claude Code's own settings), the last session continued, the office context
  function digest(r, own) {
    const parts = [r.dir ? base(r.dir) : t(own ? 'launch.ownDir' : 'launch.noDir')]
    if (r.model) parts.push(opts?.models?.find(o => o.value === r.model)?.label || r.model)
    if (r.effort) parts.push(r.effort)
    if (r.permissionMode) parts.push(r.permissionMode)
    if (!r.model && !r.effort && !r.permissionMode) parts.push(t('launch.engineOwn'))
    if (r.resume === 'continue') parts.push(t('launch.continues'))
    if (r.context) parts.push(t('launch.contextOn'))
    return parts.join(' · ')
  }

  // bypassPermissions runs everything without asking: a red line under the select says what that means (picked, it
  // scrolls into sight, and the select's description is it), and under the folded line while folded, so the settings
  // or the last start can't pick it out of sight
  function warn(reveal) {
    const on = field('permissionMode').value === 'bypassPermissions', sel = field('permissionMode')
    for (const el of form.querySelectorAll('.warn')) el.hidden = !on
    if (on) sel.setAttribute('aria-describedby', 'launch-warn')
    else sel.removeAttribute('aria-describedby')
    if (reveal && on) form.querySelector(adv.open ? '.adv .warn' : '.warn.out').scrollIntoView({ block: 'nearest' })
  }

  form.addEventListener('input', preview)
  form.addEventListener('change', preview)
  field('permissionMode').addEventListener('change', () => warn(true))
  adv.addEventListener('toggle', () => { if (adv.open) showEnd(field('dir')) })
  // every field back to the settings' (the button goes: the focus it had to the first field, not out of the dialog)
  form.querySelector('.use').onclick = ev => {
    const had = document.activeElement === ev.currentTarget
    put(settingsChoices())
    form.querySelector('.browser').hidden = true
    warn()
    preview()
    if (had) field('dir').focus()
  }
  form.querySelector('.browse').onclick = () => browse(sym, form.querySelector('.browser'), field('dir').value.trim(), reason => setStatus(reason, true), opts?.home)
  form.querySelector('.pick').onclick = () => {
    field('dir').value = form.querySelector('.browser').dataset.path
    showEnd(field('dir'))
    form.querySelector('.browser').hidden = true
    preview()
  }
  form.querySelector('.cancel').onclick = () => launch.close()
  onBackdrop(dlg, () => launch.close()) // (not a drag that selects text and lets go outside the dialog)
  form.addEventListener('submit', async ev => {
    ev.preventDefault()
    const r = request()
    if (!account && !r.name) return setStatus(t('launch.nameRequired'), true)
    form.querySelector('.go').disabled = true
    setStatus(t('launch.starting'))
    const res = await sym.launch(r)
    if (!res.ok) {
      if (FIELD_ERROR.test(res.data?.code || '')) adv.open = true // what to change is in there
      setStatus(t('launch.failed', { reason: errorText(res.data) + (res.data?.fix ? '\n' + res.data.fix : '') }), true)
      form.querySelector('.go').disabled = false
      return
    }
    pending = res.data.id
    setStatus(t('launch.startingBg'))
    launch.watch()
  })

  const launch = {
    el: dlg,
    get hidden() { return dlg.hidden },
    async open(accountId) {
      opts = await options()
      if (!opts?.enabled) return showError(opts ? offReason() : t('error.unreachable'), { title: t('errlog.oops.launchOff') })
      const acc = accountId ? sym.accountById(accountId) : null
      account = acc ? acc.id : null
      // an employee going back to work starts as it last did; a new one, or one never started from the page, as the
      // settings say (a folder that one worked in stays: a new one gets a folder of its own)
      const last = acc?.last
      fromLast = !!last
      form.querySelector('.title').textContent = acc ? t('launch.titleExisting', { name: acc.name }) : t('launch.titleNew')
      field('name').value = acc ? acc.name : ''
      field('name').readOnly = !!acc
      lookRow.hidden = !!acc // (an employee's face is changed on the employees page)
      lookPicked = false
      if (!acc) look.set(lookOf(''))
      const recent = opts.recentDirs || []
      // (one whose last choices say its own folder goes back there, wherever its session went since: the employees page
      // may have said so)
      put(last ? { dir: last.dir || acc.home || '', model: last.model || '', effort: last.effort || '', permissionMode: last.permissionMode || '', resume: '', context: !!last.context }
        : { ...settingsChoices(), dir: acc ? acc.dir || acc.home || '' : '' })
      form.querySelector('.adv-note').textContent = t(fromLast ? 'launch.fromLast' : 'launch.fromSettings')
      warn()
      form.querySelector('.recent').innerHTML = recent.map(d => `<span role="button" tabindex="0" title="${esc(d)}">${esc(base(d))}</span>`).join('')
      form.querySelectorAll('.recent span').forEach((el, i) => el.onclick = () => { field('dir').value = recent[i]; showEnd(field('dir')); preview() })
      form.querySelector('.browser').hidden = true
      setStatus('')
      form.querySelector('.go').disabled = false
      pending = null
      preview()
      // folded, unless it can't start without a folder picked (no default folder to make one in)
      adv.open = !field('dir').value.trim() && !ownDir()
      onOpen()
      dlg.hidden = false
      ;(acc ? form.querySelector('.go') : field('name')).focus() // (going back to work, Enter starts it)
      showEnd(field('dir'))
    },
    close() { dlg.hidden = true; pending = null },
    // follows the launch in the snapshot: closes the dialog once it's online, shows the reason if it failed
    watch() {
      if (!pending) return
      const l = (sym.state.launches || []).find(x => x.id === pending)
      if (!l) return
      if (l.status === 'online' || l.status === 'ended') { // ended: it came online and its process exited since
        launch.close()
        onOnline(l)
      } else if (l.status === 'failed') {
        setStatus(t('launch.failed', { reason: errorText({ code: l.errorCode, params: l.errorParams, error: l.error }) }), true)
        form.querySelector('.go').disabled = false
        pending = null
      }
    },
  }
  sym.on('snapshot', () => launch.watch())
  return launch
}

// the codes of a start refused for its folder or its options: the folded fields open
const FIELD_ERROR = /^(launch_dir_|bot_home_|launch_bad_option$|cmd_unsafe_arg$)/

// an employee's own folder's name, as the bridge makes it (slugOr in internal/bridge/git.go): letters, digits, -, _ and
// . stay, every other run of characters becomes one -, a run of dots one dot, - and . go from the ends, a final .lock
// becomes -lock, and "employee" when nothing is left
export function folderName(name) {
  let s = String(name).replace(/[^\p{L}\p{Nd}_.-]+/gu, '-').replace(/\.{2,}/g, '.').replace(/^[-.]+|[-.]+$/g, '')
  if (s.endsWith('.lock')) s = s.slice(0, -5) + '-lock'
  return s || 'employee'
}
