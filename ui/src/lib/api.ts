import { useSyncExternalStore } from 'react'

export type Kind =
  | 'received'
  | 'done'
  | 'failed'
  | 'publish'
  | 'http_in'
  | 'http_out'
  | 'integration'
  | 'error'
  | 'log'

export type Level = 'debug' | 'info' | 'warn' | 'error'

export interface LogEvent {
  seq: number
  ts: string
  container: string
  service: string
  stream: string
  format: string
  level: Level
  kind: Kind
  msg: string
  tx?: string
  action?: string
  resource?: string
  saga?: string
  step?: string
  stepN?: number
  target?: string
  status?: string
  error?: string
  payload?: unknown
  fields?: Record<string, unknown>
  noise?: boolean
  raw: string
}

export interface Container {
  id: string
  name: string
  service: string
  state: string
  status: string
  image: string
  project: string
  attached: boolean
  count: number
}

export interface Project {
  name: string
  containers: number
  running: number
  selected: boolean
  workingDir?: string
}

export interface TxSummary {
  tx: string
  first: string
  last: string
  events: number
  received: number
  failed: number
  errors: number
  services: string[]
  actions: string[]
  entry?: string
  lastStep?: string
  lastKind: Kind
}

async function get<T>(path: string): Promise<T> {
  const r = await fetch(path)
  if (!r.ok) throw new Error(`${path}: ${r.status}`)
  return r.json()
}

export const api = {
  containers: () => get<Container[]>('/api/containers'),
  txs: (q = '') => get<TxSummary[]>(`/api/tx?limit=1000&q=${encodeURIComponent(q)}`),
  tx: (id: string) => get<LogEvent[]>(`/api/tx/${encodeURIComponent(id)}`),
  saga: (id: string) => get<import('./saga-types').Saga>(`/api/saga/${encodeURIComponent(id)}`),
  projects: () => get<Project[]>('/api/projects'),
  /** lista vazia = todos os projetos compose */
  setProjects: (projects: string[]) =>
    fetch('/api/projects', { method: 'PUT', body: JSON.stringify({ projects }) }).then((r) => r.json() as Promise<string[]>),
  clear: () => fetch('/api/clear', { method: 'POST' }),
}

// ─── stream ao vivo ─────────────────────────────────────────────────────────
// Uma conexão SSE por aba, compartilhada por todas as telas.

const MAX = 30000

type Conn = 'connecting' | 'live' | 'down'

interface LiveState {
  events: LogEvent[]
  conn: Conn
  version: number
}

let state: LiveState = { events: [], conn: 'connecting', version: 0 }
const listeners = new Set<() => void>()
let started = false
let pending: LogEvent[] = []
let scheduled = false

function emit(next: Partial<LiveState>) {
  state = { ...state, ...next, version: state.version + 1 }
  listeners.forEach((l) => l())
}

function flush() {
  scheduled = false
  if (!pending.length) return
  const last = state.events.at(-1)?.seq ?? 0
  const fresh = pending.filter((e) => e.seq > last)
  pending = []
  if (!fresh.length) return
  let events = state.events.concat(fresh)
  if (events.length > MAX) events = events.slice(events.length - MAX)
  emit({ events })
}

function push(e: LogEvent) {
  pending.push(e)
  if (!scheduled) {
    scheduled = true
    // agrupa rajadas num único render
    setTimeout(flush, 120)
  }
}

function start() {
  if (started) return
  started = true
  let es: EventSource | null = null
  const connect = async () => {
    emit({ conn: 'connecting' })
    es = new EventSource('/api/stream')
    es.onopen = async () => {
      // histórico depois de abrir o stream: o que chegar no meio é deduplicado por seq
      try {
        const hist = await get<LogEvent[]>('/api/events?limit=8000&noise=1')
        const firstLive = pending[0]?.seq ?? Infinity
        const merged = hist.filter((e) => e.seq < firstLive)
        const last = state.events.at(-1)?.seq ?? 0
        const serverLast = Math.max(hist.at(-1)?.seq ?? 0, pending.at(-1)?.seq ?? 0)
        // servidor reiniciou (seq voltou): recomeça do histórico dele
        const base = serverLast < last ? [] : state.events
        const baseLast = base.at(-1)?.seq ?? 0
        emit({ events: base.concat(merged.filter((e) => e.seq > baseLast)).slice(-MAX), conn: 'live' })
      } catch {
        emit({ conn: 'live' })
      }
      flush()
    }
    es.onmessage = (m) => push(JSON.parse(m.data))
    es.onerror = () => {
      emit({ conn: 'down' })
      es?.close()
      setTimeout(connect, 2000)
    }
  }
  connect()
}

export function resetLive() {
  pending = []
  emit({ events: [] })
}

export function useLive(): LiveState {
  start()
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => state,
  )
}
