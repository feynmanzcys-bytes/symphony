// What a person allowed on the office page for the rest of a session ("Allow for this session" on a permission card).
// The mod proposes the rule with the ask (suggestRule) and keeps it once the page says yes; the page never sends a rule
// of its own. A rule lets through only what the engine itself would ask about (the mod checks that), within the
// organization's ceiling.

export type Rule = { tool: string; prefix?: string }

// the file tools one "Edit" rule covers, as Claude Code's own "allow all edits during this session" does
const EDITS = new Set(['Edit', 'MultiEdit', 'Write', 'NotebookEdit'])
// commands whose second word is what they do (npm test, git status, go vet): a rule for them takes both words
const TWO_WORDS = new Set([
  'npm', 'npx', 'pnpm', 'yarn', 'bun', 'deno', 'node', 'git', 'go', 'cargo', 'make', 'python', 'python3', 'pip', 'pip3',
  'uv', 'poetry', 'docker', 'kubectl', 'gh', 'dotnet', 'mvn', 'gradle', 'bundle', 'rake', 'rails', 'composer', 'swift',
])
// what a command line may hold to be matched by a prefix: no second command, no redirection, no substitution
const COMPOUND = /[;&|`<>\n\r]|\$\(|\$\{/

// the command of a Bash call's input, trimmed; undefined when it isn't one
function commandOf(input: unknown): string | undefined {
  const c = (input as { command?: unknown } | null)?.command
  return typeof c === 'string' ? c.trim() : undefined
}

// suggestRule is what an allow of this call could be kept as for the session: every file edit for the file tools; for
// Bash the command's first word (two for the tools in TWO_WORDS), never for a compound command; the tool itself for
// anything else (an MCP tool by its full name). Undefined when there's nothing safe to keep.
export function suggestRule(tool: string, input: unknown): Rule | undefined {
  if (EDITS.has(tool)) return { tool: 'Edit' }
  if (tool === 'Bash') {
    const command = commandOf(input)
    if (!command || COMPOUND.test(command)) return undefined
    const words = command.split(/\s+/)
    const first = words[0]
    if (!first || !/^[\w./@+-]+$/.test(first)) return undefined
    const second = words[1]
    const prefix = TWO_WORDS.has(first) && second && /^[a-z][\w:-]*$/.test(second) ? `${first} ${second}` : first
    return { tool: 'Bash', prefix }
  }
  if (tool === 'AskUserQuestion' || tool === 'ExitPlanMode') return undefined // a person's own question, never kept
  return { tool }
}

// ruleAllows says whether one of rules lets this call through: the tool is the rule's (any file tool for "Edit"), and
// for Bash the command is the prefix or starts with it and a space, and is no compound command
export function ruleAllows(rules: readonly Rule[], tool: string, input: unknown): Rule | undefined {
  for (const r of rules) {
    if (r.tool === 'Edit' && EDITS.has(tool)) return r
    if (r.tool !== tool) continue
    if (tool !== 'Bash') return r
    const command = commandOf(input)
    if (!command || !r.prefix || COMPOUND.test(command)) continue
    if (command === r.prefix || command.startsWith(r.prefix + ' ')) return r
  }
  return undefined
}

// sameRule says whether two rules are one
export function sameRule(a: Rule, b: Rule) {
  return a.tool === b.tool && (a.prefix ?? '') === (b.prefix ?? '')
}
