import {
  AlertTriangle,
  ArrowDownToLine,
  CheckCircle2,
  CornerDownLeft,
  Globe,
  type LucideIcon,
  MessageSquare,
  Plug,
  Send,
  XCircle,
} from 'lucide-react'
import type { Kind, LogEvent } from './api'

export const KIND: Record<Kind, { label: string; icon: LucideIcon; cls: string }> = {
  received: { label: 'payload', icon: ArrowDownToLine, cls: 'bg-route-soft text-route' },
  done: { label: 'concluído', icon: CheckCircle2, cls: 'bg-ok-soft text-ok' },
  failed: { label: 'falhou', icon: XCircle, cls: 'bg-fail-soft text-fail' },
  publish: { label: 'publica', icon: Send, cls: 'bg-muted text-ink-2' },
  http_in: { label: 'http in', icon: Globe, cls: 'bg-muted text-ink-2' },
  http_out: { label: 'http out', icon: CornerDownLeft, cls: 'bg-muted text-ink-2' },
  integration: { label: 'integra', icon: Plug, cls: 'bg-muted text-ink-2' },
  error: { label: 'erro', icon: AlertTriangle, cls: 'bg-fail-soft text-fail' },
  log: { label: 'log', icon: MessageSquare, cls: 'text-ink-3' },
}

export const KIND_ORDER: Kind[] = ['received', 'done', 'failed', 'publish', 'http_in', 'http_out', 'integration', 'error', 'log']

// Paleta categórica (validada para daltonismo nos dois temas). Cor = identidade do serviço.
const SLOTS = [
  'var(--c-vendor)',
  'var(--c-credential)',
  'var(--c-timeout)',
  'var(--c-ours)',
  'var(--c-infra)',
  'var(--c-concurrency)',
  'var(--c-retries)',
  'var(--m-put)',
  'var(--c-other)',
]
const assigned = new Map<string, string>()

/** Registra os serviços em ordem estável (lista de containers ordenada) para cores distintas. */
export function registerServices(names: string[]) {
  for (const n of [...names].sort()) if (!assigned.has(n)) assigned.set(n, SLOTS[assigned.size % SLOTS.length])
}

export function serviceColor(name: string) {
  if (!assigned.has(name)) assigned.set(name, SLOTS[assigned.size % SLOTS.length])
  return assigned.get(name)!
}

export function time(ts: string) {
  const d = new Date(ts)
  return (
    d.toLocaleTimeString('pt-BR', { hour12: false }) + '.' + String(d.getMilliseconds()).padStart(3, '0')
  )
}

export function dur(ms: number) {
  if (ms < 1000) return `${Math.round(ms)} ms`
  if (ms < 60000) return `${(ms / 1000).toFixed(ms < 10000 ? 2 : 1)} s`
  return `${Math.floor(ms / 60000)}m ${Math.round((ms % 60000) / 1000)}s`
}

export function ago(ts: string, now = Date.now()) {
  const s = Math.max(0, Math.round((now - new Date(ts).getTime()) / 1000))
  if (s < 60) return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}min`
  return `${Math.floor(s / 3600)}h`
}

export const shortTx = (tx: string) => tx.slice(0, 8)

export const hasPayload = (e: LogEvent) =>
  e.payload !== undefined &&
  e.payload !== null &&
  e.payload !== '' &&
  !(typeof e.payload === 'object' && Object.keys(e.payload).length === 0)

export async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    /* sem permissão: ignora */
  }
}
