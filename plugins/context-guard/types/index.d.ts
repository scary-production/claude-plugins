// Состояние context-guard в сессии.

// Сколько порогов контекста перейдено: 0 — ни одного, 1 — warn_at, 2 — wrap_at.
export type ContextGuardLevel = 0 | 1 | 2

declare module 'claude-code' {
  interface PluginState {
    'context-guard': {
      level: ContextGuardLevel
      // Окна лимитов, о которых уже предупредили: вид окна → его resetsAt.
      rateWarned: Record<string, string>
    }
  }
}
