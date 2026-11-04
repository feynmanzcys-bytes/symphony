// The colors of a fenced code block (ui/markdown.js): a small tokenizer per language, no library. A language is a table
// of rules, each a regular expression and the class its token gets, joined once into one sticky expression: where the
// tokenizer stands, the first rule that matches takes the token (so a comment comes before a string, a string before a
// word, and a word is taken whole, then told apart: a keyword, a type, a function (one a parenthesis follows) or
// nothing). What no rule names is plain. The classes: hl-k a keyword, hl-s a string, hl-c a comment, hl-n a number or
// a literal (true, nil), hl-f a function, hl-t a type (a tag's name too), hl-a an attribute (a tag's, a key, a CSS
// property, a flag, a decorator), hl-p a shell prompt, hl-x a variable, an escape or an entity, hl-i and hl-d a diff's
// added and removed lines. Operators and punctuation get no span. A string or a comment still open at the end runs to
// the end: a block streams, cut anywhere. A language it doesn't know, or none, is the code escaped as it is; so is a
// block past 64 KiB.
//
//   highlight(code, lang) → HTML (the code escaped, its tokens in spans)
import { esc } from '/asks.js'

const LIMIT = 65536

export function highlight(code, lang) {
  code = String(code ?? '')
  const def = languages[String(lang ?? '').toLowerCase()]
  if (!def || code.length > LIMIT) return esc(code)
  return def.lines ? byLines(code, def) : tokens(code, def)
}

// ---------- the tokenizer ----------
// A rule is [regex, class]; the class may be a function of (token, state, previous token, code, where the token ends)
// that answers the class, '' for plain, {to, cls} to take the token on to another end (a heredoc, to its terminator)
// or {html} to put in its place (a <script>'s content, highlighted as its own language). The state is the language's
// own ({cmd} in a shell: whether the next word is a command).
function tokens(code, def) {
  if (!def.re) compile(def)
  const { re, rules } = def
  const st = def.state ? def.state() : {}
  let out = '', plain = '', i = 0, prev = ''
  const flush = () => { if (plain) { out += esc(plain); plain = '' } }
  while (i < code.length) {
    re.lastIndex = i
    const m = re.exec(code)
    if (!m || !m[0]) { plain += code[i++]; continue } // (never, with the catch-all; kept so a rule can't hang the page)
    let k = 1
    while (m[k] === undefined) k++
    let text = m[0], c = rules[k - 1][1]
    if (typeof c === 'function') c = c(text, st, prev, code, i + text.length)
    if (c && c.to !== undefined) { text = code.slice(i, Math.max(c.to, i + text.length)); c = c.cls }
    i += text.length
    if (!c) plain += text
    else if (typeof c === 'object') { flush(); out += c.html }
    else { flush(); out += `<span class="${c}">${esc(text)}</span>` }
    if (k > 1) prev = text // (the space rule, the first, leaves it; a token taken on is its whole: a heredoc, a script)
  }
  flush()
  return out
}

// the language's rules as one expression: a group per rule, spaces first (one token, so a run of them costs one
// match), anything at all last; the rules' own groups must be (?:…)
function compile(def) {
  const rules = [[/\s+/, def.space ?? ''], ...def.rules, [/[\s\S]/, '']]
  def.rules = rules
  def.re = new RegExp(rules.map(r => `(${r[0].source})`).join('|'), 'y')
}

// a language told line by line (diff): each line's class from the first pattern it matches, the newline outside
function byLines(code, def) {
  return code.split('\n').map(line => {
    const hit = def.lines.find(([re]) => re.test(line))
    return hit && line ? `<span class="${hit[1]}">${esc(line)}</span>` : esc(line)
  }).join('\n')
}

const ahead = (re, code, at) => { re.lastIndex = at; return re.test(code) }
const CALL = /\s*\(/y, ASSIGN = /=(?!=)/y
const set = s => new Set(s.split(' '))
// a name that reads as a type: capitalized, not a constant's ALL_CAPS (a single letter is a type parameter)
const typish = w => /^[A-Z]/.test(w) && !/^[A-Z][A-Z\d_]+$/.test(w)
// the words of a C-like language: a keyword, a literal, a type (of the language's, or capitalized), a function (one of
// the language's, or a parenthesis follows); after a dot, a property: a function or nothing
function words(keywords, types, literals, functions = '', { fold = false, cap = true } = {}) {
  const K = set(keywords), T = set(types), L = set(literals), F = set(functions)
  return (text, st, prev, code, end) => {
    const w = fold ? text.toLowerCase() : text
    if (prev === '.') return ahead(CALL, code, end) ? 'hl-f' : ''
    if (K.has(w)) return 'hl-k'
    if (L.has(w)) return 'hl-n'
    if (T.has(w)) return 'hl-t'
    if (F.has(w) || ahead(CALL, code, end)) return 'hl-f'
    return cap && typish(text) ? 'hl-t' : ''
  }
}

// ---------- the pieces languages share ----------
const C_COMMENT = /\/\/[^\n]*|\/\*[\s\S]*?(?:\*\/|$)/
const DQ = /"(?:[^"\\\n]|\\[\s\S])*"?/, SQ = /'(?:[^'\\\n]|\\[\s\S])*'?/, BQ = /`(?:[^`\\]|\\[\s\S])*`?/
const C_NUMBER = /0[xX][\da-fA-F_]+|0[bB][01_]+|0[oO][0-7_]+|(?:\d[\d_]*(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?[a-zA-Z]*/
const WORD = /[A-Za-z_$][\w$]*/

// a C-like language from its word lists and what else it has (rules that go before the words)
const clike = (keywords, types, literals, functions, extra = [], strings = [DQ, SQ]) => ({
  rules: [[C_COMMENT, 'hl-c'], ...strings.map(s => [s, 'hl-s']), ...extra, [WORD, words(keywords, types, literals, functions)], [C_NUMBER, 'hl-n']],
})

const JS_KEYWORDS = 'as async await break case catch class const continue debugger default delete do else enum export extends finally for from function get if import in instanceof let new of return set static super switch this throw try typeof var void while with yield'
const TS_KEYWORDS = JS_KEYWORDS + ' abstract asserts declare implements infer interface is keyof namespace override private protected public readonly satisfies type unique'
const JS_TYPES = 'any bigint boolean never number object string symbol undefined unknown void'
const JS_LITERALS = 'false Infinity NaN null true undefined'
const js = keywords => clike(keywords, JS_TYPES, JS_LITERALS, '', [[/@[A-Za-z_$][\w$.]*/, 'hl-a']], [DQ, SQ, BQ])

const go = clike(
  'break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var',
  'any bool byte comparable complex64 complex128 error float32 float64 int int8 int16 int32 int64 rune string uint uint8 uint16 uint32 uint64 uintptr',
  'false iota nil true', 'append cap clear close complex copy delete imag len make max min new panic print println real recover', [], [DQ, SQ, /`[^`]*`?/])

const python = {
  rules: [
    [/#[^\n]*/, 'hl-c'],
    [/(?:[rRbBuUfF]{1,2})?(?:"""[\s\S]*?(?:"""|$)|'''[\s\S]*?(?:'''|$)|"(?:[^"\\\n]|\\[\s\S])*"?|'(?:[^'\\\n]|\\[\s\S])*'?)/, 'hl-s'],
    [/@[A-Za-z_][\w.]*/, 'hl-a'],
    [WORD, words('and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield',
      'bool bytearray bytes complex dict float frozenset int list memoryview object range set slice str tuple type', 'Ellipsis False None NotImplemented True')],
    [/0[xX][\da-fA-F_]+|0[bB][01_]+|0[oO][0-7_]+|(?:\d[\d_]*(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?[jJ]?/, 'hl-n'],
  ],
}

const rust = clike(
  'as async await break const continue crate dyn else enum extern fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait type unsafe use where while',
  'bool char str u8 u16 u32 u64 u128 usize i8 i16 i32 i64 i128 isize f32 f64 String Vec Option Result Box Rc Arc HashMap HashSet',
  'false None true', '', [[/#!?\[[^\]\n]*\]?/, 'hl-a'], [/'[a-z_]\w*(?!')/, 'hl-x'], [/[A-Za-z_]\w*!(?=\s*[([{])/, 'hl-f']],
  [/b?r#*"[\s\S]*?(?:"#*|$)/, /b?"(?:[^"\\]|\\[\s\S])*"?/, /b?'(?:[^'\\\n]|\\(?:u\{[^}\n]*\}|[\s\S]))'/]) // (a char literal closes: 'a alone is a lifetime)

const C_KEYWORDS = 'auto break case const continue default do else enum extern for goto if inline register restrict return sizeof static struct switch typedef union volatile while _Alignas _Alignof _Atomic _Static_assert _Thread_local'
const C_TYPES = 'bool char double FILE float int int8_t int16_t int32_t int64_t intptr_t long short signed size_t ssize_t uint8_t uint16_t uint32_t uint64_t uintptr_t unsigned void'
const PREPROCESSOR = [/(?<![^\n])[ \t]*#[ \t]*[a-z]+/, 'hl-x']
const c = clike(C_KEYWORDS, C_TYPES, 'false NULL true', '', [PREPROCESSOR])
const cpp = clike(C_KEYWORDS + ' alignas alignof and and_eq asm bitand bitor catch class compl concept consteval constexpr constinit const_cast co_await co_return co_yield decltype delete dynamic_cast explicit export final friend mutable namespace new noexcept not not_eq operator or or_eq override private protected public reinterpret_cast requires static_assert static_cast template this throw try typeid typename using virtual xor xor_eq',
  C_TYPES + ' char8_t char16_t char32_t map set shared_ptr string unique_ptr unordered_map unordered_set vector wchar_t', 'false NULL nullptr true', '', [PREPROCESSOR])
const java = clike('abstract assert break case catch class const continue default do else enum extends final finally for goto if implements import instanceof interface native new package permits private protected public record return sealed static strictfp super switch synchronized this throw throws transient try var void volatile while yield',
  'boolean Boolean byte char double Double float int Integer List long Long Map Object Optional Set short String', 'false null true', '', [[/@[A-Za-z_][\w.]*/, 'hl-a']])
const kotlin = clike('abstract actual annotation as break by catch class companion const constructor continue crossinline data do dynamic else enum expect external final finally for fun if import in infix init inline inner interface internal is lateinit noinline object open operator out override package private protected public reified return sealed super suspend tailrec this throw try typealias val var vararg when where while',
  'Any Array Boolean Byte Char Double Float Int List Long Map Nothing Set Short String Unit', 'false null true', '', [[/@[A-Za-z_][\w.]*/, 'hl-a']])
const swift = clike('actor any as associatedtype async await break case catch class continue default defer deinit do else enum extension fallthrough fileprivate for func guard if import in init inout internal is let open operator precedencegroup private protocol public repeat rethrows return self Self some static struct subscript super switch throw throws try typealias var where while',
  'Any AnyObject Array Bool Character Dictionary Double Float Int Optional Set String Void', 'false nil true', '', [[/@[A-Za-z_][\w.]*/, 'hl-a']])
const csharp = clike('abstract as async await base break case catch checked class const continue default delegate do else enum event explicit extern finally fixed for foreach get goto if implicit in init interface internal is lock namespace new operator out override params private protected public readonly record ref return sealed set sizeof stackalloc static struct switch this throw try typeof unchecked unsafe using value var virtual volatile when where while with yield',
  'bool byte char decimal double dynamic float int long nint nuint object sbyte short string uint ulong ushort void', 'false null true', '', [[/\[[A-Z][\w.]*(?:\([^)\n]*\))?\]/, 'hl-a']])

// a shell: the first word of a command is the command, what follows its arguments; a `$ ` at the start of a line is a
// prompt, as documentation shows commands. A console (shell-session) has prompts of every kind, and a line without one
// is output, left plain.
const SHELL_KEYWORDS = set('case do done elif else esac fi for function if in select then time until while'), ARGS = set('for select case in function')
const SHELL_PREFIXES = set('builtin command doas env exec nice nohup sudo time xargs')
const shellWord = (text, st, prev, code, end) => {
  if (ahead(ASSIGN, code, end)) { st.val = true; return 'hl-x' } // an assignment (its value is the next word)
  if (st.val) { st.val = false; return '' }
  if (prev === 'function') { st.cmd = false; return 'hl-f' }
  if (!st.cmd) return ''
  if (SHELL_KEYWORDS.has(text)) { if (ARGS.has(text)) st.cmd = false; return 'hl-k' }
  if (!SHELL_PREFIXES.has(text)) st.cmd = false
  return 'hl-f'
}
// a heredoc: from its terminator after << to the line that is the terminator again, one string
const heredoc = (text, st, prev, code, end) => {
  const term = text.replace(/^(['"])(.*)\1$/, '$2').replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const re = new RegExp(`\\n\\t*${term}(?=\\n|$)`, 'g')
  re.lastIndex = end
  const m = re.exec(code)
  return { to: m ? m.index + m[0].length : code.length, cls: 'hl-s' }
}
const shellRules = prompt => [
  [prompt, (t, st) => { st.cmd = true; return 'hl-p' }],
  [/(?<!\S)#[^\n]*/, 'hl-c'],
  [/(?<=(?<!<)<<-?[ \t]*)(?:'[^'\n]*'|"[^"\n]*"|\w+)/, heredoc],
  [/"(?:[^"\\]|\\[\s\S])*"?|'[^']*'?|\$'(?:[^'\\]|\\[\s\S])*'?/, 'hl-s'],
  [/\d?>{1,2}(?:&\d)?|&>{1,2}|\d?<{1,3}-?(?:&\d)?/, ''], // a redirection, before the separators
  [/\|\||&&|[|;&`]|\$?\(/, (t, st) => { st.cmd = true; return '' }],
  [/\[\[?/, (t, st) => { st.cmd = false; return '' }], // a test: what follows are its arguments
  [/\$(?:\{[^}\n]*\}?|[A-Za-z_]\w*|[@*#?$!\d-])/, 'hl-x'],
  [/(?<=\s)--?[A-Za-z\d][\w-]*/, 'hl-a'],
  [/[A-Za-z_.\/~][\w.\/~-]*/, shellWord],
  [/\d+(?![\w.-])/, 'hl-n'],
]
const shellState = () => ({ cmd: true, val: false })
const shellSpace = (t, st) => { if (t.includes('\n')) st.cmd = true; st.val = false; return '' }
const shell = { rules: shellRules(/(?<![^\n])\$ (?=\S)/), state: shellState, space: shellSpace }
const CONSOLE_PROMPT = /(?<![^\n])(?:(?:\S+ ){0,2}(?:(?!#)\S+)?(?:[$#>]|(?<!\d)%)|PS [^\n>]*>) (?=\S)/
const console_ = { state: shellState, space: shellSpace, rules: [
  ...shellRules(CONSOLE_PROMPT).slice(0, 1),
  [new RegExp(`(?<![^\\n])(?!${CONSOLE_PROMPT.source})[^\\n]*\\S[^\\n]*`), (t, st) => { st.cmd = true; return '' }], // output
  ...shellRules(CONSOLE_PROMPT).slice(1),
] }

const dockerfile = { rules: [
  [/(?<![^\n])[ \t]*#[^\n]*/, 'hl-c'],
  [/(?<![^\n])[ \t]*(?:ADD|ARG|CMD|COPY|ENTRYPOINT|ENV|EXPOSE|FROM|HEALTHCHECK|LABEL|MAINTAINER|ONBUILD|RUN|SHELL|STOPSIGNAL|USER|VOLUME|WORKDIR)(?=\s)/, 'hl-k'],
  [DQ, 'hl-s'], [/'[^'\n]*'?/, 'hl-s'],
  [/\$(?:\{[^}\n]*\}?|[A-Za-z_]\w*)/, 'hl-x'],
  [/(?<=\s)--[A-Za-z\d][\w-]*/, 'hl-a'],
] }

const json = { rules: [
  [C_COMMENT, 'hl-c'],
  [/"(?:[^"\\\n]|\\[\s\S])*"(?=\s*:)/, 'hl-a'],
  [/"(?:[^"\\\n]|\\[\s\S])*"?/, 'hl-s'],
  [/-?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?/, 'hl-n'],
  [WORD, words('', '', 'false Infinity NaN null true', '', { cap: false })],
] }

// YAML: a key before its colon, a quoted string, a number or a literal standing as the whole value, an anchor or an
// alias, a tag; a plain scalar's words stay plain
const VALUE_AT = String.raw`(?<=(?:^|\n|[:\-\[,])[ \t]+|(?:^|\n)|[\[,])`, VALUE_END = String.raw`(?=[ \t]*(?:#|\n|$|[,\]}]))`
const yaml = { rules: [
  [/(?<!\S)#[^\n]*/, 'hl-c'],
  [/(?<![^\n])(?:---|\.\.\.)(?=\s|$)/, 'hl-k'],
  [/(?<=(?:^|\n)[ \t]*(?:- +)*|[{,][ \t]*|\?[ \t]+)(?:"(?:[^"\\\n]|\\[\s\S])*"|'[^'\n]*'|[^\s"'#&*!|>{}[\],-][^\n#]*?)(?=[ \t]*:(?:[ \t]|\n|$))/, 'hl-a'],
  [/(?<![^\s[,])[&*][\w-]+/, 'hl-x'],
  [/![\w!\/:.-]*/, 'hl-t'],
  [/"(?:[^"\\]|\\[\s\S])*"?|'(?:[^']|'')*'?/, 'hl-s'],
  [new RegExp(VALUE_AT + String.raw`[-+]?(?:0x[\da-fA-F]+|0o[0-7]+|\d[\d_]*(?:\.\d*)?(?:[eE][+-]?\d+)?|\.\d+|\.(?:inf|nan|Inf|NaN|INF|NAN))` + VALUE_END), 'hl-n'],
  [new RegExp(VALUE_AT + String.raw`(?:true|false|null|yes|no|on|off|True|False|Null|Yes|No|On|Off|TRUE|FALSE|NULL|YES|NO|ON|OFF|~)` + VALUE_END), 'hl-n'],
] }

const toml = { rules: [
  [/(?<!\S)[#;][^\n]*/, 'hl-c'],
  [/(?<![^\n])[ \t]*\[\[?[^\]\n]*\]?\]?/, 'hl-t'],
  [/(?<=(?:^|\n)[ \t]*)[\w.-]+(?=[ \t]*=)/, 'hl-a'],
  [/"""[\s\S]*?(?:"""|$)|'''[\s\S]*?(?:'''|$)|"(?:[^"\\\n]|\\[\s\S])*"?|'[^'\n]*'?/, 'hl-s'],
  [/\d{4}-\d\d-\d\d(?:[T ]\d\d:\d\d:\d\d(?:\.\d+)?(?:Z|[-+]\d\d:\d\d)?)?|\d\d:\d\d:\d\d(?:\.\d+)?/, 'hl-n'],
  [/(?<![\w.-])[-+]?(?:0x[\da-fA-F_]+|0o[0-7_]+|0b[01_]+|\d[\d_]*(?:\.\d+)?(?:[eE][-+]?\d+)?|inf|nan)(?![\w.-])/, 'hl-n'],
  [/(?<![\w.-])(?:true|false)(?![\w.-])/, 'hl-n'],
] }

// HTML and XML: a tag's name, its attributes and their values (quotes outside a tag stay plain), entities; the content
// of <script> and <style> in their own languages
const html = { state: () => ({ tag: false }), rules: [
  [/<!--[\s\S]*?(?:-->|$)/, 'hl-c'],
  [/<!\[CDATA\[[\s\S]*?(?:\]\]>|$)/, ''],
  [/<![^>\n]*>?|<\?[\s\S]*?(?:\?>|$)/, 'hl-c'],
  [/(?<=<script\b[^<>]*>\s*)[\s\S]+?(?=<\/script\s*>|$)/, text => ({ html: tokens(text, languages.js) })],
  [/(?<=<style\b[^<>]*>\s*)[\s\S]+?(?=<\/style\s*>|$)/, text => ({ html: tokens(text, languages.css) })],
  [/<\/?[A-Za-z][\w:.-]*/, (t, st) => { st.tag = true; return 'hl-t' }],
  [/\/?>/, (t, st) => { st.tag = false; return '' }],
  [/[A-Za-z_:][\w:.-]*(?=\s*=)/, (t, st) => st.tag ? 'hl-a' : ''],
  [/"[^"]*"?|'[^']*'?/, (t, st) => st.tag ? 'hl-s' : ''],
  [/&(?:#\d+|#[xX][\da-fA-F]+|[A-Za-z][\w]*);/, 'hl-x'],
  [/[^\s<>&"'=\/]+/, ''], // a run of text, one token
] }

// CSS: selectors outside the braces (classes and ids as types, pseudo-classes as attributes), properties inside, the
// values' numbers with their units, colors, custom properties, functions
const css = { state: () => ({ depth: 0 }), rules: [
  [/\/\*[\s\S]*?(?:\*\/|$)/, 'hl-c'],
  [DQ, 'hl-s'], [SQ, 'hl-s'],
  [/@[\w-]+|!\s*important\b/, 'hl-k'],
  [/\{/, (t, st) => { st.depth++; return '' }],
  [/\}/, (t, st) => { if (st.depth) st.depth--; return '' }],
  [/--[\w-]+/, 'hl-x'],
  [/#[\w-]+/, (t, st) => st.depth ? (/^#[\da-fA-F]{3,8}$/.test(t) ? 'hl-n' : '') : 'hl-t'],
  [/\.[A-Za-z_-][\w-]*/, (t, st) => st.depth ? '' : 'hl-t'],
  [/[A-Za-z-][\w-]*(?=\s*\()/, 'hl-f'],
  [/[A-Za-z-][\w-]*(?=\s*:)/, (t, st) => st.depth ? 'hl-a' : ''],
  [/::?[A-Za-z-][\w-]*/, (t, st) => st.depth ? '' : 'hl-a'],
  [/[A-Za-z-][\w-]*/, ''], // a word, one token
  [/[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?(?:%|[a-zA-Z]+)?/, 'hl-n'],
] }

const sqlWords = words('add all alter analyze and as asc begin between by cascade case check column commit conflict constraint create cross database default delete desc distinct do drop else end except escape exists explain false fetch filter for foreign from full grant group having if ignore in index inner insert intersect into is join key left like limit merge natural no not nothing null of offset on or order outer over partition primary procedure recursive references replace restrict returning revoke right rollback rows schema select set show table temporary then to transaction trigger truncate union unique update upsert using vacuum values view when where window with',
    'bigint bigserial binary bit blob bool boolean bytea char character date datetime decimal double float int int2 int4 int8 integer interval json jsonb money numeric real serial smallint smallserial text time timestamp timestamptz tinyint uuid varbinary varchar xml',
    'false null true unknown', '', { fold: true, cap: false })
const NAMED_AFTER = set('database exists from function index into join procedure schema table trigger type update view')
const sql = { rules: [
  [/--[^\n]*|\/\*[\s\S]*?(?:\*\/|$)/, 'hl-c'],
  [/'(?:[^']|'')*'?/, 'hl-s'],
  [/"[^"\n]*"?|`[^`\n]*`?/, ''],
  [/\$\d+|(?<!:):[A-Za-z_]\w*|\?/, 'hl-x'],
  [WORD, (text, st, prev, code, end) => { // a table's name before its columns is no function
    const c = sqlWords(text, st, prev, code, end)
    return c === 'hl-f' && NAMED_AFTER.has(prev.toLowerCase()) ? '' : c
  }],
  [/(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?/, 'hl-n'],
] }

const diff = { lines: [
  [/^(?:diff |index |old mode|new mode|deleted file|new file|similarity |rename |copy |Binary files|\+\+\+ |--- )/, 'hl-c'],
  [/^@@/, 'hl-p'],
  [/^\+/, 'hl-i'],
  [/^-/, 'hl-d'],
] }

// the languages by the names a fence gives them
const languages = Object.create(null)
const name = (def, ...names) => { for (const n of names) languages[n] = def }
name(js(JS_KEYWORDS), 'js', 'javascript', 'jsx', 'mjs', 'cjs')
name(js(TS_KEYWORDS), 'ts', 'typescript', 'tsx', 'mts', 'cts')
name(go, 'go', 'golang')
name(python, 'py', 'python', 'python3')
name(rust, 'rs', 'rust')
name(c, 'c', 'h')
name(cpp, 'cpp', 'c++', 'cc', 'cxx', 'hpp', 'hh')
name(java, 'java')
name(kotlin, 'kt', 'kotlin', 'kts')
name(swift, 'swift')
name(csharp, 'cs', 'csharp', 'c#')
name(shell, 'sh', 'bash', 'zsh', 'shell')
name(console_, 'console', 'shell-session')
name(dockerfile, 'dockerfile', 'docker')
name(json, 'json', 'jsonc', 'json5')
name(yaml, 'yaml', 'yml')
name(toml, 'toml', 'ini', 'cfg', 'conf')
name(html, 'html', 'htm', 'xml', 'svg', 'vue')
name(css, 'css', 'scss', 'less')
name(sql, 'sql', 'mysql', 'postgres', 'postgresql', 'pgsql', 'sqlite', 'psql')
name(diff, 'diff', 'patch')
