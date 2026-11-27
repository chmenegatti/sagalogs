const TZ = 'America/Sao_Paulo'

export function fmtMs(ms: number): string {
  if (ms < 1) return '<1 ms'
  if (ms < 1000) return `${Math.round(ms)} ms`
  const s = ms / 1000
  if (s < 60) return `${s.toLocaleString('pt-BR', { maximumFractionDigits: s < 10 ? 2 : 1 })} s`
  const min = Math.floor(s / 60)
  if (min < 60) return Math.round(s % 60) ? `${min} min ${Math.round(s % 60)} s` : `${min} min`
  return `${Math.floor(min / 60)} h ${min % 60} min`
}

export function fmtClock(ts: number, withMs = true): string {
  const d = new Date(ts)
  const base = d.toLocaleTimeString('pt-BR', { timeZone: TZ, hour12: false })
  return withMs ? `${base}.${String(Math.floor(ts % 1000)).padStart(3, '0')}` : base
}

export function fmtDate(ts: number): string {
  return new Date(ts).toLocaleDateString('pt-BR', { timeZone: TZ, day: '2-digit', month: 'short', year: 'numeric' })
}

export function fmtOffset(ms: number): string {
  return `+${(ms / 1000).toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })} s`
}

export function shortTx(tx: string): string {
  return tx.length > 20 ? `${tx.slice(0, 8)}…${tx.slice(-14)}` : tx
}

/** transaction_id: UUID + sufixo de data (-AAAAMMDDhhmmss). */
export const TX_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}-\d{14}$/i

/** "13/11 10:34" no horário de Brasília */
export function fmtDayTime(ts: number): string {
  return new Date(ts).toLocaleString('pt-BR', { timeZone: TZ, day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}
