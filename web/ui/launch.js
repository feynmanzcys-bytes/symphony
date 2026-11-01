// The launch dialog (#launch), shared by every theme: starting a session from the page for a new employee (name,
// folder, model, effort, permission mode) or an existing one ("Start work": the last folder and options filled in).
// The bridge starts claude in that folder; the dialog follows the launch in the snapshot and closes once it's online.
//
//   const launch = createLaunchDialog(sym, { options, onOnline, log })
//   launch.open(accountId | null)    the dialog for an existing employee, or a new one
//   launch.close()
//   launch.watch()                   on every snapshot: closes once the launch is online, shows why it failed
//   launch.el
import { t, lang, esc, errorText, base, fill, browse, showEnd } from '/ui/common.js'

const MARKUP = `
<div class="modal" id="launch" hidden>
  <form class="panel" autocomplete="off">
    <h2 class="title"></h2>
    <label data-i18n="launch.name"></label>
    <input name="name" data-i18n-placeholder="launch.namePlaceholder" required>
    <label data-i18n="launch.dir"></label>
    <div class="row"><input name="dir" data-i18n-placeholder="launch.dirPlaceholder" required><button type="button" class="px browse" data-i18n="launch.browse"></button></div>
    <div class="browser" hidden><div class="cur"></div><ul></ul><button type="button" class="px pick" data-i18n="launch.pick"></button></div>
    <div class="recent"></div>
    <div class="grid">
      <div><label data-i18n="launch.model"></label><select name="model"></select></div>
      <div><label data-i18n="launch.effort"></label><select name="effort"></select></div>
      <div><label data-i18n="launch.permissionMode"></label><select name="permissionMode"></select></div>
      <div><label data-i18n="launch.resume"></label><select name="resume"></select></div>
    </div>
    <pre class="preview"></pre>
    <div class="status"></div>
    <div class="actions"><button type="button" class="px cancel" data-i18n="common.cancel"></button><button type="submit" class="px primary go" data-i18n="launch.go"></button></div>
  </form>
</div>`

// options: the theme's loader of /api/launch/options (it may show in its own UI why starting is off); onOnline: the
// launch came online (the theme opens the new employee's window); onOpen: the dialog opens (the theme closes its others)
export function createLaunchDialog(sym, { options = () => sym.launchOptions(true), onOnline = () => {}, onOpen = () => {}, log = () => {} } = {}) {
  const tpl = document.createElement('template')
  tpl.innerHTML = MARKUP.trim()
  const dlg = tpl.content.firstElementChild
  document.body.appendChild(dlg)
  for (const el of dlg.querySelectorAll('[data-i18n]')) el.textContent = t(el.dataset.i18n)
  for (const el of dlg.querySelectorAll('[data-i18n-placeholder]')) el.placeholder = t(el.dataset.i18nPlaceholder)
  const form = dlg.querySelector('form')
  const field = name => form.elements[name]
  let opts = null
  let pending = null // the launch we're waiting to see online
  let account = null // the existing employee going back to work; null for a new one

  // why the page can't start sessions: the mod's problem, or no launcher at all
  const offReason = () => opts?.mod ? errorText(opts.mod) : t('error.launch_disabled')

  function setStatus(text, err) {
    const el = form.querySelector('.status')
    el.textContent = text
    el.className = 'status' + (err ? ' err' : '')
  }
  function request() {
    const r = { dir: field('dir').value.trim(), model: field('model').value, effort: field('effort').value,
      permissionMode: field('permissionMode').value, resume: field('resume').value, lang }
    if (account) r.account = account
    else r.name = field('name').value.trim()
    return r
  }
  // command preview: the same arguments the bridge builds (it adds the environment variables, and --plugin-dir in development mode)
  function preview() {
    const r = request()
    const q = s => /^[\w\-./=:@]+$/.test(s) ? s : `'${s.replace(/'/g, `'\\''`)}'`
    const args = ['claude', '--name', q(field('name').value.trim() || '?')]
    if (r.model) args.push('--model', r.model)
    if (r.effort) args.push('--effort', r.effort)
    if (r.permissionMode) args.push('--permission-mode', r.permissionMode)
    if (r.resume === 'continue') args.push('--continue')
    const note = t(opts?.mod?.dev ? 'launch.previewNoteDev' : 'launch.previewNote')
    form.querySelector('.preview').textContent = `cd ${q(r.dir || '?')}\n${args.join(' ')}\n# ${note}`
  }

  form.addEventListener('input', preview)
  form.addEventListener('change', preview)
  form.querySelector('.browse').onclick = () => browse(sym, form.querySelector('.browser'), field('dir').value.trim(), reason => setStatus(reason, true), opts?.home)
  form.querySelector('.pick').onclick = () => {
    field('dir').value = form.querySelector('.browser').dataset.path
    showEnd(field('dir'))
    form.querySelector('.browser').hidden = true
    preview()
  }
  form.querySelector('.cancel').onclick = () => launch.close()
  dlg.addEventListener('click', ev => { if (ev.target === dlg) launch.close() })
  form.addEventListener('submit', async ev => {
    ev.preventDefault()
    const r = request()
    if (!account && !r.name) return setStatus(t('launch.nameRequired'), true)
    form.querySelector('.go').disabled = true
    setStatus(t('launch.starting'))
    const res = await sym.launch(r)
    if (!res.ok) {
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
      if (!opts?.enabled) return alert(opts ? offReason() : t('error.unreachable'))
      const acc = accountId ? sym.accountById(accountId) : null
      account = acc ? acc.id : null
      const last = acc?.last || {}
      form.querySelector('.title').textContent = acc ? t('launch.titleExisting', { name: acc.name }) : t('launch.titleNew')
      field('name').value = acc ? acc.name : ''
      field('name').readOnly = !!acc
      const recent = opts.recentDirs || []
      field('dir').value = last.dir || acc?.dir || recent[0] || opts.home || ''
      // a new employee starts with everything allowed, Opus and the most thinking; an old one as it last worked
      fill(field('model'), opts.models, acc ? last.model : 'opus')
      fill(field('effort'), opts.efforts, acc ? last.effort : 'max')
      fill(field('permissionMode'), opts.permissionModes, acc ? last.permissionMode : 'bypassPermissions')
      fill(field('resume'), opts.resumes, '')
      form.querySelector('.recent').innerHTML = recent.map(d => `<span title="${esc(d)}">${esc(base(d))}</span>`).join('')
      form.querySelectorAll('.recent span').forEach((el, i) => el.onclick = () => { field('dir').value = recent[i]; showEnd(field('dir')); preview() })
      form.querySelector('.browser').hidden = true
      setStatus('')
      form.querySelector('.go').disabled = false
      pending = null
      preview()
      onOpen()
      dlg.hidden = false
      ;(acc ? field('dir') : field('name')).focus()
      showEnd(field('dir'))
    },
    close() { dlg.hidden = true; pending = null },
    // follows the launch in the snapshot: closes the dialog once it's online, shows the reason if it failed
    watch() {
      if (!pending) return
      const l = (sym.state.launches || []).find(x => x.id === pending)
      if (!l) return
      if (l.status === 'online') {
        log(t('log.started', { name: l.name }))
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
