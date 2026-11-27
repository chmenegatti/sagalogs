import type { Saga } from './saga-types'

/** Sem atividade há mais que isso e sem worker final: parada (mesmo padrão do /api/stalled). */
export const STALL_MS = 15 * 60_000

export const isStalled = (s: Pick<Saga, 'status' | 'end'>, now = Date.now()) =>
  s.status === 'running' && now - s.end > STALL_MS
