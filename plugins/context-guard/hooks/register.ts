// context-guard: пороги заполнения контекста и предупреждения о лимитах подписки.
//
// Заполнение проверяется после каждого вызова инструмента и после каждого хода: долгий
// ход без пользователя тоже упирается в порог, а модель читает заметку со следующего
// своего запроса. Модели — заметка в разговор, человеку — всплывашка и строка под вводом.
import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register, SessionContextUsage, SessionRateLimit } from 'claude-code'

import type { ContextGuardLevel } from '../types'

const level = atom({ plugin: 'context-guard', key: 'level' } as const, 0 as ContextGuardLevel)
const rateWarned = atom({ plugin: 'context-guard', key: 'rateWarned' } as const, {} as Record<string, string>)

const RATE_LABELS: Record<string, string> = { five_hour: '5 часов', seven_day: '7 дней' }

type Thresholds = { warnAt: number; wrapAt: number; rateWarnAt: number }

export const register: Register = (on, options) => {
  const warnAt = numberOption(options.warn_at, 50)
  const limits: Thresholds = {
    warnAt,
    wrapAt: Math.max(numberOption(options.wrap_at, 75), warnAt),
    rateWarnAt: numberOption(options.rate_warn_at, 90),
  }

  on('session.measure', async ($, e, next) => {
    await checkContext($, e.context, limits)
    await checkRates($, e.rateLimits, limits)

    return next(e)
  })

  on('tool.call', async ($, e, next) => {
    const result = await next(e)
    const usage = await $.session.usage()
    await checkContext($, usage.context, limits)

    return result
  }).catch(($, e, next) => next(e))

  on('session.end', async ($, e, next) => {
    if (e.reason === 'clear') {
      await update($, level, () => 0)
      $.ui.status(undefined)
    }

    return next(e)
  })
}

async function checkContext($: EngineInterface, context: SessionContextUsage, limits: Thresholds): Promise<void> {
  const { warnAt, wrapAt } = limits
  const percent = context.percent
  if (percent === undefined) {
    return
  }

  const reached: ContextGuardLevel = percent >= wrapAt ? 2 : percent >= warnAt ? 1 : 0
  const before = await read($, level)
  $.ui.status(statusText(reached, percent))
  if (reached === before) {
    return
  }

  // Ниже прежнего — после /compact: порог снова сработает, когда контекст дорастёт.
  await update($, level, () => reached)
  if (reached < before) {
    return
  }

  if (reached === 2) {
    $.ui.toast(`Контекст ${percent}% — пора завершать сессию`)
    await noteForModel(
      $,
      `Контекст заполнен на ${percent}% (порог ${wrapAt}%). Заверши работу в этой сессии: ` +
        'закончи или аккуратно останови текущий шаг, запиши, что сделано, что осталось и какой ' +
        'следующий шаг, туда, где проект хранит заметки о передаче работы (если такое место есть), ' +
        'и предложи пользователю продолжить в новой сессии.',
    )
    return
  }

  $.ui.toast(`Контекст ${percent}% — дальше только текущий шаг`)
  await noteForModel(
    $,
    `Контекст заполнен на ${percent}% (порог ${warnAt}%). Доведи до конца текущий шаг и не ` +
      'начинай в этой сессии новых крупных задач; когда шаг закончен, скажи об этом пользователю.',
  )
}

async function checkRates($: EngineInterface, rates: readonly SessionRateLimit[], limits: Thresholds): Promise<void> {
  const warned = await read($, rateWarned)
  const now = await $.clock.now()
  const fresh: Record<string, string> = {}

  for (const rate of rates) {
    const label = RATE_LABELS[rate.kind]
    const window = rate.resetsAt ?? ''
    if (label === undefined || rate.percentUsed < limits.rateWarnAt || warned[rate.kind] === window) {
      continue
    }
    fresh[rate.kind] = window
    $.ui.toast(`Лимит ${label}: ${Math.floor(rate.percentUsed)}%${resetText(rate.resetsAt, now)}`)
  }

  if (Object.keys(fresh).length > 0) {
    await update($, rateWarned, current => ({ ...current, ...fresh }))
  }
}

// Заметка в разговор: модель читает её, человек не видит её как своё сообщение.
async function noteForModel($: EngineInterface, text: string): Promise<void> {
  await $.session.append({
    message: { type: 'user', content: [{ type: 'text', text: `[context-guard] ${text}` }] },
  })
}

function statusText(reached: ContextGuardLevel, percent: number): string | undefined {
  switch (reached) {
    case 2:
      return `контекст ${percent}% · пора завершать сессию`
    case 1:
      return `контекст ${percent}% · только текущий шаг`
    default:
      return undefined
  }
}

// «, сброс через 1ч 20м»; пусто, если время сброса неизвестно.
function resetText(resetsAt: string | undefined, now: number): string {
  const at = resetsAt === undefined ? Number.NaN : Date.parse(resetsAt)
  if (Number.isNaN(at)) {
    return ''
  }

  const minutes = Math.max(0, Math.round((at - now) / 60_000))
  const days = Math.floor(minutes / 1440)
  const hours = Math.floor((minutes % 1440) / 60)
  const left = days > 0 ? `${days}д ${hours}ч` : hours > 0 ? `${hours}ч ${minutes % 60}м` : `${minutes}м`

  return `, сброс через ${left}`
}

function numberOption(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}
