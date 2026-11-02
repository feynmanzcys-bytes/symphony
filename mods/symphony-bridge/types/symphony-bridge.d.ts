// symphony-bridge's own contract (plugin.json "types"): the values it keeps in the session's state.

declare module 'claude-code' {
  interface PluginState {
    'symphony-bridge': {
      // the session's token and bridge, held by the engine for the session: the mod takes SYMPHONY_TOKEN out of the
      // environment once the session started, and a module reloaded by /reload-plugins reads them here
      office: { token: string; bridge: string }
      // what the office page allowed for the rest of the session ("Allow for this session"): a tool, and for Bash the
      // commands that start with prefix
      rules: { tool: string; prefix?: string }[]
    }
  }
}
