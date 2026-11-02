// Getting the symphony program when it isn't installed: the release of the mod's own version, from GitHub, checked
// against that release's checksums.txt, unpacked into the mod's own folder (bin/). The mod's file API takes text only
// and up to 4 MiB, so the download, the check and the unpacking are done by the programs every supported system has:
// curl, sha256sum / shasum / certutil, tar (Windows 10 and later ship curl.exe and tar.exe, which reads zip).

import type { EngineInterface } from 'claude-code'

export const RELEASES = 'https://github.com/feynmanzcys-bytes/symphony/releases/download'

export type Target = { os: 'linux' | 'darwin' | 'windows'; arch: 'amd64' | 'arm64' }

// targetOf reads the system and the processor from `uname -sm` (Linux, macOS) or, where there's no uname, from
// Windows's PROCESSOR_ARCHITEW6432 / PROCESSOR_ARCHITECTURE; undefined for anything the releases don't build
export function targetOf(uname: string | undefined, winArch: string | undefined): Target | undefined {
  if (uname) {
    const [system, machine = ''] = uname.trim().split(/\s+/)
    const os = system === 'Linux' ? 'linux' : system === 'Darwin' ? 'darwin' : undefined
    const arch = /^(x86_64|amd64)$/i.test(machine) ? 'amd64' : /^(aarch64|arm64)$/i.test(machine) ? 'arm64' : undefined
    return os && arch ? { os, arch } : undefined
  }
  const arch = winArch?.toUpperCase()
  if (arch === 'AMD64') return { os: 'windows', arch: 'amd64' }
  if (arch === 'ARM64') return { os: 'windows', arch: 'arm64' }
  return undefined
}

// the archive a release has for target (the names carry no version: .github/release.sh)
export function archiveOf(t: Target) {
  return `symphony_${t.os}_${t.arch}.${t.os === 'windows' ? 'zip' : 'tar.gz'}`
}

// sumFor is archive's SHA-256 in a checksums.txt (sha256sum's format: the hex, spaces, an optional *, the name)
export function sumFor(checksums: string, archive: string): string | undefined {
  for (const line of checksums.split(/\r?\n/)) {
    const m = /^([0-9a-fA-F]{64})\s+\*?(\S+)$/.exec(line.trim())
    if (m && m[2] === archive) return m[1].toLowerCase()
  }
  return undefined
}

// hashIn finds the SHA-256 in what sha256sum, shasum -a 256 or certutil -hashfile printed (older certutil puts a
// space between every byte)
export function hashIn(output: string): string | undefined {
  for (const raw of output.split(/\r?\n/)) {
    const line = raw.trim()
    const joined = line.replace(/\s+/g, '')
    if (/^[0-9a-fA-F]{64}$/.test(joined)) return joined.toLowerCase()
    const first = line.split(/\s+/)[0] ?? ''
    if (/^[0-9a-fA-F]{64}$/.test(first)) return first.toLowerCase()
  }
  return undefined
}

// the last line a failed program said, for the message
function said(r: { stdout: string; stderr: string; exitCode: number }) {
  const last = (s: string) =>
    s
      .split(/\r?\n/)
      .map(l => l.trim())
      .filter(Boolean)
      .at(-1)
  return (last(r.stderr) ?? last(r.stdout) ?? `exit status ${r.exitCode}`).slice(0, 200)
}

export type Plan = { version: string; target: Target }

// download gets the release's program for plan into dir and answers its path, or why not. A program already there (an
// earlier session's download of this version: bin/ is per version for an install from GitHub) is used as it is.
export async function download($: EngineInterface, plan: Plan, dir: string): Promise<{ exe?: string; reason?: string }> {
  const { target, version } = plan
  const windows = target.os === 'windows'
  // Windows's own separator in every path a program gets: cmd's del reads a "/" as the start of a switch
  const sep = windows ? '\\' : '/'
  if (windows) dir = dir.replace(/\//g, '\\')
  const exe = `${dir}${sep}symphony${windows ? '.exe' : ''}`
  if (await $.fs.stat(exe).then(() => true, () => false)) return { exe }
  const archive = archiveOf(target)
  const base = `${RELEASES}/v${version}`
  const stamp = `${Date.now().toString(36)}${Math.floor(Math.random() * 1e9).toString(36)}`
  const part = `${dir}${sep}${archive}.${stamp}.part`
  const run = async (argv: string[], timeoutMs = 120_000) => {
    try {
      return await $.process.run(argv, { cwd: $.plugin.root, timeoutMs })
    } catch (err) {
      const why = err instanceof Error ? err.message : String(err)
      return { exitCode: 127, stdout: '', stderr: `${argv[0]}: ${why}`, isStdoutTruncated: false, isStderrTruncated: false }
    }
  }
  const remove = (path: string) => (windows ? run(['cmd', '/d', '/c', 'del', '/f', '/q', path]) : run(['rm', '-rf', path]))
  // https only, also after GitHub's redirect to its download host
  const curl = [windows ? 'curl.exe' : 'curl', '-fsSL', '--proto', '=https', '--proto-redir', '=https']
  if (!windows) {
    const made = await run(['mkdir', '-p', dir])
    if (made.exitCode !== 0) return { reason: said(made) }
  }
  const got = await run([...curl, '--create-dirs', '-o', part, `${base}/${archive}`], 600_000)
  if (got.exitCode !== 0) {
    await remove(part)
    return { reason: said(got) }
  }
  const sums = await run([...curl, `${base}/checksums.txt`])
  const want = sums.exitCode === 0 ? sumFor(sums.stdout, archive) : undefined
  const hashed = await run(
    windows ? ['certutil', '-hashfile', part, 'SHA256'] : target.os === 'darwin' ? ['shasum', '-a', '256', part] : ['sha256sum', part],
  )
  const have = hashed.exitCode === 0 ? hashIn(hashed.stdout) : undefined
  if (!want || !have || want !== have) {
    await remove(part)
    if (sums.exitCode !== 0) return { reason: said(sums) }
    if (!want) return { reason: `checksums.txt has no line for ${archive}` }
    if (!have) return { reason: said(hashed) }
    return { reason: `${archive} doesn't match its checksum` }
  }
  if (windows) {
    // tar.exe reads the zip; only when there's no program yet (a running one can't be written over)
    const out = await run(['tar', '-xf', part, '-C', dir, 'symphony.exe'])
    await remove(part)
    if (out.exitCode !== 0) return { reason: said(out) }
    return { exe }
  }
  // unpacked into a folder of its own, then moved into place in one rename: a session starting it meanwhile never
  // runs half a program
  const unpack = `${dir}/.unpack-${stamp}`
  const steps = [
    ['mkdir', '-p', unpack],
    ['tar', '-xzf', part, '-C', unpack, 'symphony'],
    ['chmod', '755', `${unpack}/symphony`],
    ['mv', '-f', `${unpack}/symphony`, exe],
  ]
  for (const step of steps) {
    const r = await run(step)
    if (r.exitCode !== 0) {
      await remove(unpack)
      await remove(part)
      return { reason: said(r) }
    }
  }
  await remove(unpack)
  await remove(part)
  return { exe }
}
