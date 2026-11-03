// The page's own settings, kept in this browser (localStorage) for every theme, and told to whoever listens when one
// changes (in this tab: the settings dialog sets them):
//
//   pref(name, fallback)     what name is set to, else fallback
//   setPref(name, value)     set it (null takes it away) and tell the listeners
//   onPref(name, fn)         fn(value) whenever name is set
const heard = new Map()

export function pref(name, fallback = null) {
  try { return localStorage.getItem(name) ?? fallback } catch { return fallback }
}

export function setPref(name, value) {
  try {
    if (value === null || value === undefined) localStorage.removeItem(name)
    else localStorage.setItem(name, String(value))
  } catch {}
  for (const fn of heard.get(name) ?? []) fn(value ?? null)
}

export function onPref(name, fn) {
  if (!heard.has(name)) heard.set(name, new Set())
  heard.get(name).add(fn)
}

// the names every theme shares
export const STEPS_OPEN = 'symphony.steps' // 'open': the steps between answers start unfolded in the employee window
export const FAULTS_SEEN = 'symphony.faultsSeen' // when the newest entry of the error log was that its user has seen
