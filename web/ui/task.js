// The collaborative tasks' UI, shared by every theme: the task dialog (#task: what the team should do, its members —
// employees ticked in order, advisors made for this task —, whether the first one leads) and the task window
// (#taskwin: what it's on, who is in it — a click opens a member's window —, the team chat with the mention picker,
// and the button to end it; a failed one says why and can be dismissed). The bridge starts a session for each member
// in a git worktree of its own and sends each the kickoff.
//
//   const tasks = createTaskUI(sym, { options, roomLabel, memberAvatar, youAvatar, isMemberOpen, onOpenMember, onWindowOpen, onWindowClose, onDialogOpen })
//   tasks.openDialog()
//   tasks.closeDialog()
//   tasks.openWindow(id, data)      data: what the bridge answered, until the snapshot has the task; once the task ended
//                                   and left the snapshot, the window says so until it is closed
//   tasks.closeWindow()
//   tasks.render()                  draw the window again (on every snapshot and message)
//   tasks.windowFor                 the task whose window is open, or null
//   tasks.dialog, tasks.win         the elements
import { t, lang, esc, errorText, base, lower, dot, fill, browse, showEnd, statusText, richText, TASK_MARK, GRAY_MARK, button, deferWhilePressed, setHTML, pointAt, displayName } from '/ui/common.js'
import { dialog as asDialog } from '/ui/a11y.js'
import { putErrorBox, setText, showError, onBackdrop } from '/ui/errors.js'
import { markdown } from '/ui/markdown.js'

const MAX_MEMBERS = 5 // the bridge's cap (taskMaxMembers)

const DIALOG = `
<div class="modal" id="task" hidden>
  <form class="panel" autocomplete="off">
    <h2 class="title"></h2>
    <label for="task-desc" data-i18n="task.description"></label>
    <textarea id="task-desc" name="desc" rows="4" data-i18n-placeholder="task.descriptionPlaceholder"></textarea>
    <label id="task-members" data-i18n="task.members"></label>
    <div class="emps" role="group" aria-labelledby="task-members"></div>
    <div class="advs" role="group" aria-labelledby="task-members"></div>
    <button type="button" class="px add-adv" data-i18n="task.addAdvisor" data-i18n-title="task.advisorTitle"></button>
    <label class="check"><input type="checkbox" name="leader" checked><span data-i18n="task.leader"></span></label>
    <label for="task-mode" data-i18n="launch.permissionMode"></label>
    <select id="task-mode" name="permissionMode"></select>
    <div class="warn" data-i18n="launch.bypassWarning" hidden></div>
    <div class="foot">
      <div class="status"></div>
      <div class="actions"><span class="count"></span><button type="button" class="px cancel" data-i18n="common.cancel"></button><button type="submit" class="px primary go" data-i18n="task.go"></button></div>
    </div>
  </form>
</div>`

const WINDOW = `
<div class="modal" id="taskwin" hidden>
  <div class="panel">
    <button type="button" class="close"></button>
    <div class="top"><h2></h2><span class="status"><i class="lamp" aria-hidden="true"></i><span class="tx"></span></span><div class="actions"></div></div>
    <div class="desc"></div>
    <div class="mode" data-i18n="task.flat" hidden></div>
    <div class="err" hidden></div>
    <div class="members"></div>
    <div class="tchat">
      <h3 data-i18n="task.chat"></h3>
      <div class="tlist"></div>
      <form class="tsay" autocomplete="off" role="none">
        <div class="tbox"><textarea rows="1" data-i18n-aria-label="task.chat" aria-autocomplete="list" aria-haspopup="listbox" aria-controls="task-pick"></textarea><div class="tpick" id="task-pick" role="listbox" data-i18n-aria-label="task.mentionList" hidden></div></div>
        <button type="submit" class="px primary" data-i18n="chat.send"></button>
      </form>
      <div class="tstatus"></div>
    </div>
  </div>
</div>`

function mount(markup) {
  const tpl = document.createElement('template')
  tpl.innerHTML = markup.trim()
  const el = tpl.content.firstElementChild
  document.body.appendChild(el)
  for (const x of el.querySelectorAll('[data-i18n]')) x.textContent = t(x.dataset.i18n)
  for (const x of el.querySelectorAll('[data-i18n-placeholder]')) x.placeholder = t(x.dataset.i18nPlaceholder)
  for (const x of el.querySelectorAll('[data-i18n-title]')) x.title = t(x.dataset.i18nTitle)
  for (const x of el.querySelectorAll('[data-i18n-aria-label]')) x.setAttribute('aria-label', t(x.dataset.i18nAriaLabel))
  return el
}

// the names a message goes by; one you sent from a task's chat has no sender
export const fromYou = m => m.origin === 'page' && !m.fromName
export const msgFrom = m => m.fromName || '?' // (yours carry no name: the bubble's side says it)

// why a task failed: its first failure in this page's language, then the bridge's own words when they say more (every
// failure of its cleanup, one a line, git's among them)
function failedWhy(task) {
  const said = t('task.failedWhy', { reason: errorText({ code: task.errorCode, params: task.errorParams, error: task.error }) })
  const all = String(task.error || '').trim()
  return all && !said.includes(all) ? said + '\n\n' + all : said
}

export function createTaskUI(sym, {
  options = () => sym.launchOptions(true),
  roomLabel = task => task.id,
  memberAvatar = () => 'none',
  youAvatar = () => 'none',
  isMemberOpen = () => false,
  onOpenMember = () => {},
  onWindowOpen = () => {},
  onWindowClose = () => {},
  onDialogOpen = () => {},
  mentionName = name => name, // how a member is named in the team chat (its session name, or what the theme shows)
} = {}) {
  const dialog = mount(DIALOG), win = mount(WINDOW)
  const form = dialog.querySelector('form')
  asDialog(dialog, form.querySelector('.title'))
  asDialog(win, win.querySelector('.top h2')) // (a region where the theme lays it out: Coworkers)
  let opts = null
  let picks = [] // the members, in order: rows of the dialog (an employee ticked, or an advisor added)
  let posting = false
  const offReason = () => opts?.mod ? errorText(opts.mod) : t('error.launch_disabled')

  // ---------- the dialog ----------
  // the status line: what goes on, or an error in a box (a failed start is in the error log already: the bridge keeps
  // it, so the page doesn't report it again)
  function setStatus(text, err) {
    const el = form.querySelector('.status')
    el.className = 'status' + (err ? ' err' : '')
    if (err) { putErrorBox(el, text, { report: false }); el.scrollIntoView({ block: 'nearest' }) }
    else el.textContent = text
  }
  // the rows an error is about, outlined in red: advisors without a name or a folder (code 'incomplete'), with a name
  // that's taken or too long, or in a folder that's missing, and the member the error names
  function markBad(code, params = {}) {
    form.querySelectorAll('.bad').forEach(el => el.classList.remove('bad'))
    const taken = new Set(sym.state.accounts.map(a => lower(a.name))), seen = new Set()
    for (const row of picks) {
      const name = row.querySelector('input[name=name]'), dir = row.querySelector('input[name=dir]')
      const who = lower(name ? name.value.trim() : row.querySelector('.nm').textContent)
      const fields = !name ? [] : code === 'incomplete' ? [name, dir].filter(f => !f.value.trim())
        : code === 'account_name_taken' ? (taken.has(who) || seen.has(who) ? [name] : [])
        : code === 'account_bad_name' ? ([...name.value.trim()].length > 40 || !who ? [name] : [])
        : code === 'launch_dir_missing' || code === 'launch_dir_not_absolute' ? (params.dir ? dir.value.trim() === params.dir : !dir.value.trim().match(/^([\/\\]|[A-Za-z]:)/)) ? [dir] : []
        : []
      seen.add(who)
      if (fields.length || (params.name && lower(params.name) === who)) { row.classList.add('bad'); fields.forEach(f => f.classList.add('bad')) }
    }
    form.querySelector('.bad')?.scrollIntoView({ block: 'nearest' })
  }
  // an advisor: a one-off member made for this task with the launch form's fields; it joins the team at the end
  function addAdvisor() {
    const row = document.createElement('div'), recent = opts?.recentDirs || []
    row.className = 'adv'
    row.innerHTML = `<div class="line"><b class="ord"></b><span class="kind">${esc(t('task.advisor'))}</span><span class="lead-tag">${esc(t('task.lead'))}</span>` +
      `<input name="name" placeholder="${esc(t('task.advisorName'))}" aria-label="${esc(t('task.advisorName'))}"><button type="button" class="px rm" title="${esc(t('task.remove'))}" aria-label="${esc(t('task.remove'))}"></button></div>` +
      `<div class="row"><input name="dir" placeholder="${esc(t('launch.dirPlaceholder'))}" aria-label="${esc(t('launch.dir'))}"><button type="button" class="px browse">${esc(t('launch.browse'))}</button>` +
      `<select name="model" title="${esc(t('launch.model'))}" aria-label="${esc(t('launch.model'))}"></select><select name="effort" title="${esc(t('launch.effort'))}" aria-label="${esc(t('launch.effort'))}"></select></div>` +
      `<div class="browser" hidden><div class="cur"></div><ul></ul><button type="button" class="px pick">${esc(t('launch.pick'))}</button></div>` +
      `<div class="recent">${recent.map(d => `<span role="button" tabindex="0" title="${esc(d)}">${esc(base(d))}</span>`).join('')}</div>`
    const dir = row.querySelector('[name=dir]'), box = row.querySelector('.browser')
    dir.value = recent[0] || opts?.home || ''
    row.querySelector('.browse').onclick = () => browse(sym, box, dir.value.trim(), reason => setStatus(reason, true), opts?.home)
    box.querySelector('.pick').onclick = () => { dir.value = box.dataset.path; box.hidden = true; showEnd(dir) }
    row.querySelectorAll('.recent span').forEach((el, i) => el.onclick = () => { dir.value = recent[i]; showEnd(dir) })
    // as the settings say a new employee starts (Claude Code's own when they say nothing)
    fill(row.querySelector('[name=model]'), opts?.models || [], opts?.defaults?.model ?? '')
    fill(row.querySelector('[name=effort]'), opts?.efforts || [], opts?.defaults?.effort ?? '')
    row.querySelector('.rm').onclick = () => { picks = picks.filter(x => x !== row); row.remove(); renderPicks() }
    form.querySelector('.advs').appendChild(row)
    picks.push(row)
    renderPicks()
    row.querySelector('[name=name]').focus()
    showEnd(dir)
  }
  // the members' numbers, the lead's tag, the count; Start only for 2 to MAX_MEMBERS members
  function renderPicks() {
    const lead = form.elements.leader.checked
    for (const row of form.querySelectorAll('.mem, .adv')) {
      const i = picks.indexOf(row)
      row.classList.toggle('on', i >= 0)
      row.classList.toggle('lead', i === 0 && lead)
      row.classList.toggle('over', i >= MAX_MEMBERS) // past the team's size
      row.querySelector('.ord').textContent = i >= 0 ? String(i + 1) : ''
    }
    const n = picks.length, count = form.querySelector('.count')
    count.textContent = t('task.count', { n, max: MAX_MEMBERS })
    count.classList.toggle('over', n > MAX_MEMBERS)
    form.querySelector('.go').disabled = posting || n < 2 || n > MAX_MEMBERS
  }
  // bypassPermissions runs everything without asking: a red line under the select says what that means. It's the last
  // field, so picked, the dialog scrolls down to it, above the buttons that stay at the bottom
  function warn(reveal) {
    const el = form.querySelector('.warn')
    el.hidden = form.elements.permissionMode.value !== 'bypassPermissions'
    if (reveal && !el.hidden) form.scrollTop = form.scrollHeight
  }
  form.querySelector('.add-adv').onclick = addAdvisor
  form.elements.leader.onchange = renderPicks
  form.elements.permissionMode.onchange = () => warn(true)
  form.addEventListener('input', ev => { ev.target.classList.remove('bad'); if (!ev.target.closest('.adv')?.querySelector('input.bad')) ev.target.closest('.adv')?.classList.remove('bad') })
  form.querySelector('.cancel').onclick = () => tasks.closeDialog()
  onBackdrop(dialog, () => tasks.closeDialog()) // (not a drag that selects text and lets go outside the dialog)
  form.addEventListener('submit', async ev => {
    ev.preventDefault()
    const title = form.elements.desc.value.trim()
    if (!title) return setStatus(t('task.descriptionRequired'), true)
    const members = picks.map(row => {
      const m = { model: row.querySelector('[name=model]').value, effort: row.querySelector('[name=effort]').value }
      if (row.dataset.account) m.account = row.dataset.account
      else m.advisor = { name: row.querySelector('[name=name]').value.trim(), dir: row.querySelector('[name=dir]').value.trim() }
      return m
    })
    if (members.some(m => m.advisor && (!m.advisor.name || !m.advisor.dir))) { markBad('incomplete'); return setStatus(t('task.advisorIncomplete'), true) }
    markBad('')
    const body = { title, leader: form.elements.leader.checked, permissionMode: form.elements.permissionMode.value, lang, members }
    posting = true
    renderPicks()
    setStatus(t('launch.starting'))
    const res = await sym.tasks.create(body)
    posting = false
    if (!res.ok) {
      if (res.data?.code) markBad(res.data.code, res.data.params)
      setStatus(t('task.failed', { reason: res.data ? errorText(res.data) + (res.data.fix ? '\n' + res.data.fix : '') : `${t('error.unknown')} (HTTP ${res.status})` }), true)
      if (!dialog.hidden) renderPicks()
      return
    }
    tasks.closeDialog()
    tasks.openWindow(res.data.id, res.data)
  })

  // ---------- the window ----------
  let windowFor = null, windowData = null, shown = {}
  const tList = win.querySelector('.tlist'), tPick = win.querySelector('.tpick'), tSay = win.querySelector('.tsay')
  const tBox = tSay.querySelector('textarea'), tStatus = win.querySelector('.tstatus')
  let chatFor = null // the task the input's draft and status line belong to
  let chatStick = false // you just sent: follow the list down when it comes back, wherever it was scrolled
  win.querySelector('.close').onclick = () => tasks.closeWindow()
  onBackdrop(win, () => tasks.closeWindow())

  // (not while a pointer is down: a member's row replaced under a press would lose its click)
  const render = deferWhilePressed(() => {
    if (!windowFor) return
    const known = sym.taskById(windowFor)
    if (known) windowData = null
    let task = known ?? (windowData?.id === windowFor ? windowData : null)
    // it ended and left the snapshot (only tasks.json says so): the window shows it ended, as it last was, until it is
    // closed; a failed task dismissed just goes
    if (!task && shown.task?.status && shown.task.status !== 'failed') task = windowData = { ...shown.task, status: 'ended' }
    if (!task) return tasks.closeWindow()
    shown.task = task
    // (each written only when it changed: what is selected in it stays through the redraws, every snapshot)
    setText(win.querySelector('h2'), `${task.id} · ${roomLabel(task)}`)
    const st = win.querySelector('.top .status')
    setText(st.querySelector('.tx'), t('task.status.' + task.status))
    st.style.setProperty('--c', TASK_MARK[task.status] ?? GRAY_MARK)
    setText(win.querySelector('.desc'), task.title)
    win.querySelector('.mode').hidden = !!task.leader
    // a failed task: why, all of it (the bridge lists every failure of the cleanup, and its error log has it already)
    const err = win.querySelector('.err')
    err.hidden = task.status !== 'failed'
    putErrorBox(err, err.hidden ? '' : failedWhy(task), { report: false })
    // the members: state, name, lead and advisor tags, where it works, its model; a click opens its window
    const rows = task.members.map((m, i) => {
      const s = sym.sessionById(m.session), online = sym.isOnline(s), state = !online ? 'ended' : sym.asking(m.session) ? 'waiting' : s.state
      const runs = [s?.model || m.model, s?.effort || m.effort].filter(Boolean).join(' · ')
      const open = m.session && isMemberOpen(task, m) // its window is beside this one
      return `<div class="mrow${m.session ? '' : ' wait'}${open ? ' open' : ''}" data-i="${i}"${m.session ? ' role="button" tabindex="0"' : ''}${open ? ' aria-current="true"' : ''}>${dot(state)}<b>${esc(displayName(m.name, task))}</b>` +
        (task.leader && m.account === task.leader ? `<span class="badge lead">${esc(t('task.lead'))}</span>` : '') +
        (m.advisor ? `<span class="badge adv" title="${esc(t('task.advisorTitle'))}">${esc(t('task.advisor'))}</span>` : '') +
        `<span class="where" title="${esc([m.dir, m.worktree].filter(Boolean).join('\n'))}">${esc(base(m.dir))} · ${esc(m.branch || '')}</span>` +
        (runs ? `<span class="chip">${esc(runs)}</span>` : '') +
        `<span class="st">${esc(!m.session ? t('task.memberStarting') : online ? statusText(sym, s) : t('task.memberEnded'))}</span></div>`
    }).join('')
    if (rows !== shown.members) { shown.members = rows; setHTML(win.querySelector('.members'), rows) } // (a row with the focus keeps it)
    const stick = renderChat(task)
    const btnKey = task.id + '|' + task.status
    if (btnKey !== shown.btns) {
      shown.btns = btnKey
      const btns = win.querySelector('.actions')
      btns.innerHTML = ''
      if (task.status === 'running') btns.appendChild(button(t('task.end'), 'alert end', async () => {
        if (!confirm(t('task.endConfirm', { id: task.id }))) return
        const r = await sym.tasks.end(task.id)
        if (!r.ok) showError(errorText(r.data), { title: t('errlog.oops.endTask') })
      }))
      if (task.status === 'failed') btns.appendChild(button(t('task.dismiss'), 'dismiss', async () => {
        const r = await sym.tasks.forget(task.id)
        if (!r.ok) showError(errorText(r.data), { title: t('errlog.oops.dismissTask') })
        else tasks.closeWindow()
      }, t('task.dismissTitle')))
    }
    win.hidden = false
    if (shown.grow) { shown.grow = false; growBox() }
    if (stick) tList.scrollTop = tList.scrollHeight // once it shows: a hidden list has no height to scroll
  })

  // The task chat: the messages between the task's members and yours to them, oldest first, each with its sender's
  // figure. "@bob-T1 …" goes to that member only, anything else to every member online; the bridge sends them on.
  // the members a message names go before its words, in its first paragraph when it has one (a member's is Markdown)
  const withMentions = (to, body) => to && body.startsWith('<p>') ? '<p>' + to + body.slice(3) : to + body
  function msgHTML(task, m) {
    const me = fromYou(m), who = me ? null : task.members.find(x => (x.session && x.session === m.from) || x.name === m.fromName)
    const names = m.toNames?.length ? m.toNames : m.toName ? [m.toName] : [] // one or several members, or everyone
    const missed = m.missed || [] // of those, the ones that were offline: struck through
    const to = names.length ? `<b class="to">${names.map(n => missed.includes(n) ? `<s title="${esc(t('task.missed', { name: n }))}">@${esc(mentionName(n, task))}</s>` : '@' + esc(mentionName(n, task))).join(' ')}</b> ` : ''
    return `<div class="tmsg${me ? ' me' : ''}"><span class="av" style="background-image:${esc(me ? youAvatar() : memberAvatar(who))}"></span>` +
      `<div class="bub">${me ? '' : `<div class="hd"><b>${esc(displayName(msgFrom(m), task))}</b></div>`}` +
      `<div class="body${me ? '' : ' md'}">${withMentions(to, me ? richText(m.text) : markdown(m.text))}</div><time>${esc(new Date(m.time).toLocaleTimeString(lang))}</time></div></div>`
  }
  // redrawn when the message log or the members' figures change; says whether the list should be scrolled to its end
  // what of a message the chat shows, so a message elsewhere in the office (another task's, between desks) changes
  // nothing here, and a new one of this task's is added below the others rather than all of them drawn again
  const msgSig = m => `${m.id}|${m.from}|${m.fromName}|${m.toName}|${(m.toNames || []).join(',')}|${(m.missed || []).join(',')}|${m.text?.length}`
  function renderChat(task) {
    let stick = false
    const figures = task.members.map(m => { const s = sym.sessionById(m.session); return `${m.name}|${m.session}|${s?.model}|${s?.effort}` }).join(',')
    const list = sym.state.messages.filter(m => m.task === task.id)
    const sigs = list.map(msgSig)
    const head = `${task.id}|${figures}|${lang}`
    const old = shown.chatSigs || []
    // the same task and figures, and what was shown is still the start of the list: only the new ones are drawn
    const appendOnly = shown.chatHead === head && old.length > 0 && old.length <= sigs.length && old.every((x, i) => x === sigs[i])
    if (shown.chatHead !== head || sigs.length !== old.length || !appendOnly) {
      const follow = !shown.chatSigs || chatStick || tList.scrollTop + tList.clientHeight >= tList.scrollHeight - 40
      if (appendOnly) {
        tList.insertAdjacentHTML('beforeend', list.slice(old.length).map(m => msgHTML(task, m)).join(''))
      } else {
        // it follows new messages down unless you scrolled up to read; it opens at the end
        const top = tList.scrollTop
        tList.innerHTML = list.length ? list.map(m => msgHTML(task, m)).join('') : `<div class="empty">${esc(t('task.chatEmpty'))}</div>`
        tList.scrollTop = top
      }
      shown.chatHead = head
      shown.chatSigs = sigs
      stick = follow
      if (stick) chatStick = false
    }
    const running = task.status === 'running'
    if (!running) closePick()
    tBox.disabled = !running
    tSay.querySelector('button').disabled = !running
    const ph = running ? t('task.chatPlaceholder', { name: task.members[0] ? mentionName(task.members[0].name, task) : '' }) : t('task.chatOff')
    if (tBox.placeholder !== ph) { tBox.placeholder = ph; shown.grow = true }
    return stick
  }
  // as tall as what's in it, the placeholder too (up to a few lines, then it scrolls); a hidden one has no height to take
  function growBox() {
    if (win.hidden) { shown.grow = true; return }
    tBox.style.height = ''
    tBox.style.height = Math.min(tBox.scrollHeight + 4, 140) + 'px'
  }
  // The mention picker: an @ typed opens a list of the task's members over the input, narrowed by what follows it;
  // arrows move, Enter or Tab (or a click) puts in "@name ", Esc, a space or no match closes it until the next @. For
  // screen readers the input names the list it fills in (aria-autocomplete, aria-controls) and the member picked
  // (aria-activedescendant), as a combobox would: a textarea can't be one
  let pick = null // { at, q, sel }: where the @ being typed is, what follows it, the highlighted name
  let pickShut = -1 // the @ Esc closed the picker for
  function pickNames() {
    const task = sym.taskById(windowFor) ?? windowData
    const q = pick ? lower(pick.q) : ''
    return (task?.members ?? []).filter(m => lower(m.name).startsWith(q) || lower(mentionName(m.name, task)).startsWith(q))
  }
  function closePick() { pick = null; tPick.hidden = true; pointAt(tBox, null) }
  function markPick() { // the highlighted member, for the eye and for screen readers
    tPick.querySelectorAll('.hit').forEach((h, i) => { h.classList.toggle('on', i === pick.sel); h.setAttribute('aria-selected', String(i === pick.sel)) })
    pointAt(tBox, 'task-pick-' + pick.sel)
  }
  function updatePick() {
    const before = tBox.value.slice(0, tBox.selectionStart), hit = tBox.selectionStart === tBox.selectionEnd && /(?:^|\s)@([^\s@]*)$/.exec(before)
    if (!hit || tBox.disabled) { pickShut = -1; return closePick() }
    const at = before.length - hit[1].length - 1
    if (at === pickShut) return closePick()
    pick = { at, q: hit[1], sel: pick?.at === at ? pick.sel : 0 }
    const names = pickNames()
    if (!names.length) return closePick()
    pick.sel = Math.max(0, Math.min(pick.sel, names.length - 1))
    const task = sym.taskById(windowFor) ?? windowData
    setHTML(tPick, names.map((m, i) => { // (in place, and the name apart from its line: as the search boxes' lists)
      const s = sym.sessionById(m.session), online = sym.isOnline(s)
      return `<div class="hit${i === pick.sel ? ' on' : ''}" data-name="${esc(m.name)}" role="option" id="task-pick-${i}" aria-selected="${i === pick.sel}"><i class="av" style="background-image:${esc(memberAvatar(m))}" aria-hidden="true"></i>${dot(online ? s.state : 'ended')}<b>${esc(mentionName(m.name, task))}</b> ` +
        `<small>${esc(!m.session ? t('task.memberStarting') : online ? statusText(sym, s) : t('task.memberEnded'))}</small></div>`
    }).join(''), true)
    tPick.hidden = false
    markPick()
  }
  function putPick(name) {
    const end = tBox.selectionStart, at = pick.at, task = sym.taskById(windowFor) ?? windowData
    tBox.setRangeText('@' + mentionName(name, task) + ' ', at, end, 'end')
    closePick(); growBox()
  }
  tPick.addEventListener('mousedown', ev => { // mousedown, so the box keeps its focus
    const row = ev.target.closest('.hit')
    if (!row || !pick) return
    ev.preventDefault()
    putPick(row.dataset.name)
  })
  tBox.addEventListener('input', () => { growBox(); updatePick() })
  tBox.addEventListener('click', updatePick)
  tBox.addEventListener('blur', closePick)
  tBox.addEventListener('keydown', ev => {
    if (pick && !ev.isComposing) {
      const names = pickNames()
      if (ev.key === 'ArrowDown' || ev.key === 'ArrowUp') {
        pick.sel = (pick.sel + (ev.key === 'ArrowDown' ? 1 : names.length - 1)) % names.length
        markPick()
      } else if ((ev.key === 'Enter' || ev.key === 'Tab') && names[pick.sel]) putPick(names[pick.sel].name)
      else if (ev.key === 'Escape') { pickShut = pick.at; closePick(); ev.stopPropagation() } // the picker only, not the window
      else return
      ev.preventDefault()
      return
    }
    if (ev.key === 'Enter' && !ev.shiftKey && !ev.isComposing) { ev.preventDefault(); tSay.requestSubmit() }
  })
  tSay.addEventListener('submit', async ev => {
    ev.preventDefault()
    const id = windowFor, text = tBox.value.trim()
    if (!id || !text) return
    tBox.value = ''; growBox(); closePick()
    tStatus.textContent = ''; tStatus.classList.remove('note')
    chatStick = true
    const r = await sym.tasks.say(id, text)
    if (id !== chatFor) return
    if (r.ok) { // sent, though maybe not to every member it named: say who was offline
      const missed = r.data?.missed
      if (missed?.length && id === chatFor && !tStatus.textContent) {
        tStatus.textContent = t('task.sentMissed', { names: missed.join(', ') }); tStatus.classList.add('note')
      }
      return
    }
    chatStick = false
    if (!tBox.value) { tBox.value = text; growBox() } // back in the box, to fix and send again
    tStatus.textContent = t('task.sayFailed', { reason: errorText(r.data) })
  })
  win.querySelector('.members').addEventListener('click', ev => {
    const row = ev.target.closest('.mrow'), task = sym.taskById(windowFor) ?? windowData, m = row && task?.members[row.dataset.i]
    if (m?.session) onOpenMember(task, m)
  })

  const tasks = {
    dialog, win,
    get windowFor() { return windowFor },
    get dialogHidden() { return dialog.hidden },
    // the dialog: the employees not in a running task, with a folder to work in, can be picked
    async openDialog() {
      opts = await options()
      if (!opts?.enabled) return showError(opts ? offReason() : t('error.unreachable'), { title: t('errlog.oops.launchOff') })
      form.querySelector('.title').textContent = t('task.titleNew')
      form.elements.desc.value = ''
      form.elements.leader.checked = true
      fill(form.elements.permissionMode, opts.permissionModes, opts.defaults?.permissionMode ?? '') // the settings' (Claude Code's own when they say nothing)
      warn()
      // the employees: one in a running task, or without a folder to work in yet, can't be picked
      const busy = new Map()
      for (const task of sym.state.tasks) if (task.status !== 'failed') for (const m of task.members) busy.set(m.account, task.id)
      picks = []
      const list = form.querySelector('.emps')
      list.innerHTML = ''
      for (const acc of sym.state.accounts.filter(a => !a.advisor)) {
        const s = sym.sessionOf(acc), dir = s?.cwd || acc.last?.dir || acc.dir || '', inTask = busy.get(acc.id), off = !!inTask || !dir
        const row = document.createElement('div')
        row.className = 'mem' + (off ? ' off' : '')
        row.dataset.account = acc.id
        row.innerHTML = `<label><input type="checkbox"${off ? ' disabled' : ''}><b class="ord"></b>` +
          `${dot(s && s.state !== 'ended' ? s.state : 'ended', statusText(sym, s && s.state !== 'ended' ? s : null))}<span class="nm">${esc(acc.name)}</span><span class="lead-tag">${esc(t('task.lead'))}</span>` +
          `<small title="${esc(dir)}">${esc(inTask ? t('task.inTask', { id: inTask }) : dir ? base(dir) : t('task.noDir'))}</small></label>` +
          `<select name="model" title="${esc(t('launch.model'))}" aria-label="${esc(t('launch.model'))}"></select><select name="effort" title="${esc(t('launch.effort'))}" aria-label="${esc(t('launch.effort'))}"></select>`
        // as it last worked, or, never started from the page, as the settings say
        const from = acc.last || opts.defaults || {}
        fill(row.querySelector('[name=model]'), opts.models, from.model ?? '')
        fill(row.querySelector('[name=effort]'), opts.efforts, from.effort ?? '')
        row.querySelector('input').onchange = ev => { picks = picks.filter(x => x !== row); if (ev.target.checked) picks.push(row); renderPicks() }
        list.appendChild(row)
      }
      form.querySelector('.advs').innerHTML = ''
      setStatus('')
      posting = false
      renderPicks()
      onDialogOpen()
      dialog.hidden = false
      form.elements.desc.focus()
    },
    closeDialog() { dialog.hidden = true },
    // the window; until the snapshot has a task just started, it shows what the bridge answered
    openWindow(id, data = null) {
      if (id !== chatFor) { chatFor = id; tBox.value = ''; tStatus.textContent = '' } // a draft stays with its task
      windowFor = id; windowData = data; shown = { grow: true }
      onWindowOpen(id)
      render()
    },
    closeWindow() {
      const was = !win.hidden
      win.hidden = true; windowFor = null; closePick()
      if (was) onWindowClose()
    },
    render,
    growBox,
  }
  sym.on('snapshot', () => { if (windowFor) render() })
  sym.on('message', () => { if (windowFor) render() })
  sym.on('messages', () => { if (windowFor) render() })
  return tasks
}
