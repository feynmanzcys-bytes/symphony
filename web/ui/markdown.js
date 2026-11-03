// Markdown, for what the model writes (its answers, its thinking, the messages between sessions): the blocks GitHub's
// flavor has (headings, paragraphs, lists, nested, numbered, with tasks, quotes, fenced code, tables, rules) and the
// inline marks (code, bold, italics, strikethrough, links, bare addresses). No library: the page loads nothing from
// elsewhere. Everything is escaped first, so raw HTML shows as text; a link goes out only to http, https or mailto, in a
// new tab, without the page as its referrer; an image is a link to it (the page shows nothing from other sites). A line
// break inside a paragraph stays a break, as a chat shows what was written. A text still streaming renders as far as it
// goes: a fence not yet closed is code to the end.
//
//   markdown(text) → HTML (a fragment of blocks)
import { esc } from '/asks.js'

const LIST = /^( {0,3})([-*+]|\d{1,9}[.)])(?:[ \t]+|$)/
const FENCE = /^ {0,3}(`{3,}|~{3,})[ \t]*([^\s`]*)[^`]*$/
const HEADING = /^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$/
const RULE = /^ {0,3}([-*_])(?:[ \t]*\1){2,}[ \t]*$/
const QUOTE = /^ {0,3}>/
const DELIMITER = /^ {0,3}\|?[ \t]*:?-+:?[ \t]*(?:\|[ \t]*:?-+:?[ \t]*)*\|?[ \t]*$/
const blank = l => /^[ \t]*$/.test(l)
const indent = l => l.match(/^ */)[0].length
const tableAt = (lines, i) => lines[i].includes('|') && i + 1 < lines.length && DELIMITER.test(lines[i + 1]) && lines[i + 1].includes('-')
// a line that starts a block of its own, so it ends a paragraph
const opens = (lines, i) => {
  const l = lines[i]
  return FENCE.test(l) || HEADING.test(l) || RULE.test(l) || QUOTE.test(l) || tableAt(lines, i) || (LIST.test(l) && !blank(l.replace(LIST, '')))
}

export function markdown(src) {
  const lines = String(src ?? '').replace(/\r\n?/g, '\n').replace(/\t/g, '    ').split('\n')
  return blocks(lines, false)
}

// the blocks of lines; tight: a list item's, whose paragraphs need no <p> of their own
function blocks(lines, tight) {
  const out = []
  let i = 0
  while (i < lines.length) {
    const line = lines[i]
    if (blank(line)) { i++; continue }
    let m
    if ((m = line.match(FENCE))) { // fenced code, to its closing fence (or the end, while it streams)
      const close = new RegExp('^ {0,3}' + (m[1][0] === '`' ? '`' : '~') + '{' + m[1].length + ',}[ \\t]*$'), body = []
      for (i++; i < lines.length && !close.test(lines[i]); i++) body.push(lines[i])
      i++
      out.push(`<pre${m[2] ? ` data-lang="${esc(m[2])}"` : ''}><code>${esc(body.join('\n'))}</code></pre>`)
      continue
    }
    if ((m = line.match(HEADING))) { // # .. ######, as h3 .. h6: the page's own headings come first
      const level = Math.min(6, m[1].length + 2)
      out.push(`<h${level}>${inline(m[2] ?? '')}</h${level}>`)
      i++
      continue
    }
    if (RULE.test(line)) { out.push('<hr>'); i++; continue }
    if (QUOTE.test(line)) {
      const body = []
      for (; i < lines.length && QUOTE.test(lines[i]); i++) body.push(lines[i].replace(/^ {0,3}> ?/, ''))
      out.push(`<blockquote>${blocks(body, false)}</blockquote>`)
      continue
    }
    if (tableAt(lines, i)) { i = table(lines, i, out); continue }
    if (LIST.test(line)) { i = list(lines, i, out); continue }
    // a paragraph: up to a blank line or a line that starts another block
    const body = [line.trim()]
    for (i++; i < lines.length && !blank(lines[i]) && !opens(lines, i); i++) body.push(lines[i].trim())
    const html = inline(body.join('\n')).replace(/\n/g, '<br>')
    out.push(tight ? html : `<p>${html}</p>`)
  }
  return out.join('')
}

// a list from line i (its items at the first one's indent, of its kind); a deeper line, or one indented into an item,
// is the item's: a nested list, a second paragraph, code
function list(lines, i, out) {
  const first = lines[i].match(LIST), base = first[1].length, ordered = /\d/.test(first[2])
  const items = []
  let loose = false
  while (i < lines.length) {
    const m = lines[i].match(LIST)
    if (!m || m[1].length !== base || /\d/.test(m[2]) !== ordered) break
    const pad = m[0].length // where the item's own text starts
    const body = [lines[i].slice(pad)]
    for (i++; i < lines.length; i++) {
      const l = lines[i]
      if (blank(l)) { // a blank line: the item goes on when what follows is indented into it
        let j = i
        while (j < lines.length && blank(lines[j])) j++
        if (j < lines.length && (indent(lines[j]) >= pad || (LIST.test(lines[j]) && indent(lines[j]) > base))) {
          body.push(...lines.slice(i, j).map(() => ''))
          i = j - 1
          if (indent(lines[j]) >= pad && !LIST.test(lines[j])) loose = true
          continue
        }
        if (j < lines.length && LIST.test(lines[j]) && indent(lines[j]) === base) loose = true // a blank line between items
        break
      }
      if (indent(l) >= pad || (LIST.test(l) && indent(l) > base)) { body.push(l.slice(Math.min(indent(l), pad))); continue }
      if (LIST.test(l) || opens(lines, i)) break
      body.push(l.trim()) // a lazy line: the paragraph goes on
    }
    items.push(body)
    while (i < lines.length && blank(lines[i]) && i + 1 < lines.length && LIST.test(lines[i + 1]) && indent(lines[i + 1]) === base) i++
  }
  const start = ordered ? parseInt(first[2], 10) : 1
  out.push((ordered ? `<ol${start !== 1 ? ` start="${start}"` : ''}>` : '<ul>') + items.map(body => {
    const task = !ordered && body[0].match(/^\[([ xX])\][ \t]+/)
    if (task) body[0] = body[0].slice(task[0].length)
    const mark = task ? `<input type="checkbox" disabled${task[1] === ' ' ? '' : ' checked'}> ` : '' // (read as done or not)
    return `<li${task ? ` class="task${task[1] === ' ' ? '' : ' done'}"` : ''}>${mark}${blocks(body, !loose)}</li>`
  }).join('') + (ordered ? '</ol>' : '</ul>'))
  return i
}

// a table from line i: the header, the delimiter (where each column leans), the rows up to a line without a pipe
function table(lines, i, out) {
  const cells = row => split(row.trim().replace(/^\|/, '')).map(c => c.trim())
  const head = cells(lines[i]), lean = cells(lines[i + 1]).map(d => /^:-+:$/.test(d) ? 'center' : /-:$/.test(d) ? 'right' : /^:-/.test(d) ? 'left' : '')
  const cell = (tag, text, k) => `<${tag}${lean[k] ? ` style="text-align:${lean[k]}"` : ''}>${inline(text ?? '')}</${tag}>`
  const rows = []
  for (i += 2; i < lines.length && !blank(lines[i]) && lines[i].includes('|') && !opens(lines, i); i++) rows.push(cells(lines[i]))
  out.push(`<div class="table"><table><thead><tr>${head.map((c, k) => cell('th', c, k)).join('')}</tr></thead>` +
    (rows.length ? `<tbody>${rows.map(r => `<tr>${head.map((_, k) => cell('td', r[k], k)).join('')}</tr>`).join('')}</tbody>` : '') + '</table></div>')
  return i
}

// a row's cells: split at its pipes, but an escaped one (\|, which inline() makes a plain pipe) or one in code (`a|b`,
// kept whole, more forgiving than GitHub); a last pipe ends the row
function split(row) {
  const cells = []
  let cell = ''
  for (let k = 0; k < row.length; k++) {
    const c = row[k]
    if (c === '\\' && row[k + 1] === '|') { cell += '\\|'; k++; continue }
    if (c === '`') {
      const run = row.slice(k).match(/^`+/)[0], end = row.indexOf(run, k + run.length)
      if (end > 0) { cell += row.slice(k, end + run.length); k = end + run.length - 1; continue }
    }
    if (c === '|') { cells.push(cell); cell = '' }
    else cell += c
  }
  if (cell.trim() || !cells.length) cells.push(cell)
  return cells
}

// ---------- inline ----------
// The literal pieces (code, escaped characters, links) are put aside first, the rest is escaped and marked, then they
// go back where they were.
function inline(text) {
  const kept = []
  const keep = html => `\u0000${kept.push(html) - 1}\u0000`
  text = text.replace(/(`+)([^`]|[^`][\s\S]*?[^`])\1(?!`)/g, (m, ticks, code) => keep(`<code>${esc(code.length > 2 && code[0] === ' ' && code.at(-1) === ' ' ? code.slice(1, -1) : code)}</code>`))
  text = text.replace(/\\([\\`*_{}[\]()#+\-.!|~<>"])/g, (m, c) => keep(esc(c)))
  text = text.replace(/<((?:https?|mailto):[^\s<>]+)>/gi, (m, url) => keep(link(url, esc(url))))
  text = text.replace(/(!?)\[([^\]\n]*)\]\([ \t]*<?([^\s<>()]*(?:\([^\s<>()]*\)[^\s<>()]*)*)>?(?:[ \t]+(?:"[^"\n]*"|'[^'\n]*'))?[ \t]*\)/g,
    (m, bang, label, url) => keep(link(url, marks(esc(label)) || esc(url), !!bang)))
  let html = esc(text)
  html = html.replace(/\bhttps?:\/\/(?:(?!&quot;|&lt;|&gt;)[^\s<\u0000])+/g, url => {
    const tail = url.match(/[.,;:!?'*_~)\]]+$/)?.[0] ?? '' // the sentence's own punctuation stays out
    const href = tail ? url.slice(0, -tail.length) : url
    return keep(link(href.replace(/&amp;/g, '&'), href)) + tail
  })
  html = marks(html)
  for (let n = 0; n < 4 && html.includes('\u0000'); n++) html = html.replace(/\u0000(\d+)\u0000/g, (m, k) => kept[k])
  return html
}
// bold, italics, strikethrough, on escaped text (an underscore inside a word is no mark: snake_case stays)
const marks = html => html
  .replace(/\*\*(?=\S)([\s\S]*?\S)\*\*/g, '<strong>$1</strong>')
  .replace(/(^|[^\w])__(?=\S)([\s\S]*?\S)__(?!\w)/g, '$1<strong>$2</strong>')
  .replace(/(^|[^*\w\\])\*(?=[^\s*])([^*\n]*?[^\s*])\*(?!\*)/g, '$1<em>$2</em>')
  .replace(/(^|[^_\w])_(?=[^\s_])([^_\n]*?[^\s_])_(?![_\w])/g, '$1<em>$2</em>')
  .replace(/~~(?=\S)([\s\S]*?\S)~~/g, '<del>$1</del>')
// a link out of the page: http, https and mailto only (anything else is its words); an image, a link to it
function link(url, label, image = false) {
  if (!/^(?:https?:\/\/|mailto:)/i.test(url)) return label
  return `<a href="${esc(url)}" target="_blank" rel="noopener noreferrer"${image ? ' class="img"' : ''}>${label}</a>`
}
