import { expect, mock, test } from 'claude-code/testing'
import type { Engine } from 'claude-code/testing'
import type { On, SessionRateLimit } from 'claude-code'

const OPTIONS = { options: { warn_at: 40, wrap_at: 70, rate_warn_at: 90 } }
const NOW = Date.parse('2026-10-18T12:00:00Z')

// Мир под плагином: часы и замер; всплывашки и строка под вводом записываются в массивы.
function world(on: On) {
  const toasts: string[] = []
  const statuses: (string | undefined)[] = []
  mock.clock(on, { now: NOW })
  on('ui.toast', (_$, e) => {
    toasts.push(e.text)
    return { value: undefined }
  })
  on('ui.status', (_$, e) => {
    statuses.push(e.text)
    return { value: undefined }
  })
  // Замер разговора идёт снизу от движка: тест отвечает за него сам.
  on('session.measure', (_$, e) => ({ changed: e.changed }))

  return { toasts, statuses, session: mock.session(on) }
}

function measure($: Engine, percent: number | undefined, rateLimits: SessionRateLimit[] = []) {
  return $.session.measure({
    context: { window: 1_000_000, percent, tokens: percent === undefined ? undefined : percent * 10_000 },
    rateLimits,
    changed: ['context'],
  })
}

function notes(session: ReturnType<typeof mock.session>): string[] {
  return session.appended().map(row => JSON.stringify(row.message.content))
}

test('ниже первого порога молчит', OPTIONS, async ($, on) => {
  const w = world(on)
  await measure($, 39)
  await measure($, undefined)

  expect(notes(w.session)).toEqual([])
  expect(w.toasts).toEqual([])
})

test('первый порог: одна заметка модели и одна всплывашка', OPTIONS, async ($, on) => {
  const w = world(on)
  await measure($, 41)
  await measure($, 55)

  const rows = notes(w.session)
  expect(rows.length).toBe(1)
  expect(rows[0]).toContain('порог 40%')
  expect(rows[0]).toContain('не начинай')
  expect(w.toasts).toEqual(['Контекст 41% — дальше только текущий шаг'])
  expect(w.statuses.at(-1)).toBe('контекст 55% · только текущий шаг')
})

test('второй порог: просит завершить сессию', OPTIONS, async ($, on) => {
  const w = world(on)
  await measure($, 45)
  await measure($, 72)

  const rows = notes(w.session)
  expect(rows.length).toBe(2)
  expect(rows[1]).toContain('порог 70%')
  expect(rows[1]).toContain('Заверши работу')
  expect(w.statuses.at(-1)).toBe('контекст 72% · пора завершать сессию')
})

test('после сжатия контекста порог срабатывает снова', OPTIONS, async ($, on) => {
  const w = world(on)
  await measure($, 50)
  await measure($, 12)
  await measure($, 44)

  expect(notes(w.session).length).toBe(2)
  expect(w.statuses).toContain(undefined)
})

test('лимит окна — одна всплывашка на окно', OPTIONS, async ($, on) => {
  const w = world(on)
  const fiveHour = { kind: 'five_hour', percentUsed: 91.5, resetsAt: '2026-10-18T13:20:00Z' }
  await measure($, 10, [fiveHour])
  await measure($, 10, [{ ...fiveHour, percentUsed: 95 }])
  await measure($, 10, [{ ...fiveHour, percentUsed: 92, resetsAt: '2026-10-18T18:20:00Z' }])
  await measure($, 10, [{ kind: 'seven_day', percentUsed: 50, resetsAt: '2026-10-20T12:00:00Z' }])

  expect(w.toasts).toEqual(['Лимит 5 часов: 91%, сброс через 1ч 20м', 'Лимит 5 часов: 92%, сброс через 6ч 20м'])
})

test('посреди хода: порог ловится после вызова инструмента', OPTIONS, async ($, on) => {
  const w = world(on)
  on('session.usage', () => ({ value: { startedAt: 0, context: { window: 1_000_000, percent: 46 }, rateLimits: [] } }))
  on('tool.call', () => ({ result: 'ok' }))
  await $.tool.call({ tool: 'mcp__fixture__noop' })

  expect(notes(w.session).length).toBe(1)
  expect(w.statuses.at(-1)).toBe('контекст 46% · только текущий шаг')
})
