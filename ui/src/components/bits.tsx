import { Link } from '@tanstack/react-router'
import { Check, Copy } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import type { Kind, LogEvent } from '@/lib/api'
import { KIND, copy, serviceColor, shortTx } from '@/lib/meta'
import { cn } from '@/lib/utils'

export function ServiceTag({ name, className }: { name: string; className?: string }) {
  return (
    <span className={cn('inline-flex min-w-0 items-center gap-1.5 font-mono text-[12px] text-ink', className)}>
      <span className="size-2 shrink-0 rounded-[3px]" style={{ background: serviceColor(name) }} aria-hidden />
      <span className="truncate">{name}</span>
    </span>
  )
}

export function KindPill({ kind, className }: { kind: Kind; className?: string }) {
  const k = KIND[kind]
  const Icon = k.icon
  return (
    <span
      className={cn(
        'inline-flex h-5 shrink-0 items-center gap-1 rounded px-1.5 text-[11px] font-medium tracking-wide uppercase',
        k.cls,
        className,
      )}
    >
      <Icon className="size-3" aria-hidden />
      {k.label}
    </span>
  )
}

export function TxChip({ tx, className }: { tx: string; className?: string }) {
  return (
    <Link
      to="/tx"
      search={{ id: tx }}
      onClick={(e) => e.stopPropagation()}
      title={`transação ${tx}`}
      className={cn(
        'inline-flex h-5 shrink-0 items-center rounded border border-rule bg-sheet px-1.5 font-mono text-[11px] text-ink-2 hover:border-route hover:text-route',
        className,
      )}
    >
      {shortTx(tx)}
    </Link>
  )
}

export function StepTag({ e }: { e: Pick<LogEvent, 'step' | 'stepN'> }) {
  if (!e.step) return null
  return (
    <span className="inline-flex h-5 shrink-0 items-center gap-1 rounded bg-paper px-1.5 font-mono text-[11px] text-ink-2">
      {e.stepN !== undefined && <span className="font-semibold text-ink">{e.stepN}</span>}
      {e.step}
    </span>
  )
}

export function CopyBtn({ text, label = 'copiar' }: { text: string; label?: string }) {
  const [ok, setOk] = useState(false)
  return (
    <button
      type="button"
      onClick={(ev) => {
        ev.stopPropagation()
        copy(text)
        setOk(true)
        setTimeout(() => setOk(false), 1200)
      }}
      className="inline-flex h-6 items-center gap-1 rounded px-1.5 text-[11px] text-ink-3 hover:bg-paper hover:text-ink"
    >
      {ok ? <Check className="size-3 text-ok" /> : <Copy className="size-3" />}
      {ok ? 'copiado' : label}
    </button>
  )
}

export function Toggle({
  on,
  onChange,
  children,
  className,
}: {
  on: boolean
  onChange: (v: boolean) => void
  children: ReactNode
  className?: string
}) {
  return (
    <button
      type="button"
      aria-pressed={on}
      onClick={() => onChange(!on)}
      className={cn(
        'inline-flex h-7 items-center gap-1.5 rounded-md border px-2 text-[12px] transition-colors',
        on ? 'border-route bg-route-soft text-route' : 'border-rule bg-sheet text-ink-2 hover:text-ink',
        className,
      )}
    >
      {children}
    </button>
  )
}

export const pretty = (v: unknown) => (typeof v === 'string' ? v : JSON.stringify(v, null, 2))
