// The theme picker, shared by every theme: a <select> of the office page's themes (GET /api/themes), the current one
// selected; picking one opens it (/?theme=<name>, which the bridge remembers in a cookie).
import { t, lang } from '/ui/common.js'

export async function themePicker(sym) {
  const sel = document.createElement('select')
  sel.className = 'theme'
  sel.title = t('common.theme')
  sel.setAttribute('aria-label', t('common.theme'))
  const r = await sym.themes()
  const list = r.ok ? r.data.themes || [] : []
  const current = r.ok ? r.data.current : document.documentElement.dataset.theme
  for (const th of list) sel.add(new Option(th.title?.[lang] ?? th.title?.en ?? th.name, th.name, false, th.name === current))
  sel.onchange = () => {
    const url = new URL(location.href)
    url.pathname = '/'
    url.searchParams.set('theme', sel.value)
    location.href = url
  }
  return sel
}
