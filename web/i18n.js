// Translations for the office and approval pages. Each language is one file, locales/<lang>.json,
// with the same keys as en.json. t() falls back to English, then to the key itself.
//
// The language comes from ?lang= in the address, then the one picked in the page (saved in
// localStorage), then the browser's languages, and English when none of them is supported.

export const LANGS = [
  ['en', 'English'], ['zh-CN', '简体中文'], ['zh-TW', '繁體中文'], ['ja', '日本語'], ['ko', '한국어'],
  ['es', 'Español'], ['fr', 'Français'], ['de', 'Deutsch'], ['pt-BR', 'Português (Brasil)'], ['ru', 'Русский'],
]
const STORE = 'symphony.lang'

// pickLang maps language tags ("zh-HK", "pt", "en-US") to the first supported language, or English.
export function pickLang(wanted) {
  for (const tag of wanted || []) {
    const parts = String(tag).toLowerCase().replace(/_/g, '-').split('-')
    if (parts[0] === 'zh') return parts.slice(1).some(p => ['hant', 'tw', 'hk', 'mo'].includes(p)) ? 'zh-TW' : 'zh-CN'
    if (parts[0] === 'pt') return 'pt-BR'
    const hit = LANGS.find(([l]) => l.toLowerCase() === parts[0])
    if (hit) return hit[0]
  }
  return 'en'
}

function stored() {
  try { return localStorage.getItem(STORE) } catch { return null }
}

export const lang = pickLang([new URLSearchParams(location.search).get('lang'), stored(),
  ...(navigator.languages || [navigator.language])].filter(Boolean))
document.documentElement.lang = lang

async function load(path) {
  const r = await fetch(path)
  if (!r.ok) throw new Error(`${path}: ${r.status}`)
  return r.json()
}
// The shared catalogs (/locales/) hold the keys every page uses; a theme (data-theme on <html>) may add its own in
// /t/<theme>/locales/<lang>.json, which go over the shared ones. A theme without catalogs is fine (404).
const theme = document.documentElement.dataset.theme
const own = l => Promise.all([load(`/locales/${l}.json`), theme ? load(`/t/${theme}/locales/${l}.json`).catch(() => null) : null])
  .then(([shared, mine]) => ({ ...shared, ...mine }))
const [en, mine] = await Promise.all([own('en'), lang === 'en' ? null : own(lang).catch(() => null)])
const dict = mine ?? en

// Keys a page asked for that its language lacks; the end-to-end test checks that this stays empty.
window.__i18nMissing = window.__i18nMissing || []

// t returns the text for key with each {name} replaced by params[name]. The result is plain text:
// put it in with textContent, or through esc() when building HTML.
export function t(key, params) {
  let s = dict[key]
  if (s === undefined) {
    if (!window.__i18nMissing.includes(key)) window.__i18nMissing.push(key)
    s = en[key] ?? key
  }
  return params ? s.replace(/\{(\w+)\}/g, (m, k) => (k in params ? String(params[k]) : m)) : s
}

// errorText is the message for an API error body ({error, code, params}) in this language.
export function errorText(body) {
  if (body?.code && (('error.' + body.code) in en)) return t('error.' + body.code, body.params || {})
  return body?.error || t('error.unknown')
}

// fmtTime formats a timestamp the way this language writes dates and times.
export const fmtTime = when => new Date(when).toLocaleString(lang)

// applyI18n fills the elements marked in the HTML: data-i18n (text), data-i18n-placeholder and data-i18n-title.
export function applyI18n(root = document) {
  for (const el of root.querySelectorAll('[data-i18n]')) el.textContent = t(el.dataset.i18n)
  for (const el of root.querySelectorAll('[data-i18n-placeholder]')) el.placeholder = t(el.dataset.i18nPlaceholder)
  for (const el of root.querySelectorAll('[data-i18n-title]')) el.title = t(el.dataset.i18nTitle)
}

// langPicker is a <select> of the languages; picking one saves it and reloads the page in it.
export function langPicker() {
  const sel = document.createElement('select')
  sel.className = 'lang'
  sel.title = t('common.language')
  sel.setAttribute('aria-label', t('common.language'))
  for (const [value, name] of LANGS) sel.add(new Option(name, value, false, value === lang))
  sel.onchange = () => {
    try { localStorage.setItem(STORE, sel.value) } catch {}
    const url = new URL(location.href)
    url.searchParams.delete('lang')
    location.href = url
  }
  return sel
}
