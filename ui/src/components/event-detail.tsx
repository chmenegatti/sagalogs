import { Link } from '@tanstack/react-router'
import { ArrowRight, X } from 'lucide-react'
import { type ReactNode, useEffect } from 'react'
import type { LogEvent } from '@/lib/api'
import { hasPayload, time } from '@/lib/meta'
import { CopyBtn, KindPill, ServiceTag, StepTag, pretty } from './bits'
import { Payload } from './json-tree'

/** Painel lateral com tudo de um evento: payload, campos, linha crua. */
export function EventDetail({ e, onClose }: { e: LogEvent; onClose: () => void }) {
  useEffect(() => {
    const k = (ev: KeyboardEvent) => ev.key === 'Escape' && onClose()
    window.addEventListener('keydown', k)
    return () => window.removeEventListener('keydown', k)
  }, [onClose])

  const fields = { ...e.fields }
  for (const k of ['state', 'worker', 'body', 'integration_args', 'method_args']) delete fields[k]

  return (
    <aside className="flex h-full min-h-0 flex-col border-l border-rule bg-sheet">
      <header className="flex items-start gap-3 border-b border-rule px-4 py-3">
        <div className="min-w-0 flex-1 space-y-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <KindPill kind={e.kind} />
            <ServiceTag name={e.container} />
            <span className="font-mono text-[12px] text-ink-3">{time(e.ts)}</span>
          </div>
          <p className="text-[14px] font-medium break-words text-ink">{e.msg}</p>
          <div className="flex flex-wrap items-center gap-1.5 text-[12px] text-ink-2">
            {e.action && <span className="rounded bg-paper px-1.5 font-mono">{e.action}</span>}
            {e.resource && <span className="rounded bg-paper px-1.5 font-mono">{e.resource}</span>}
            {e.saga && <span className="rounded bg-paper px-1.5 font-mono">{e.saga}</span>}
            <StepTag e={e} />
            {e.target && <span className="font-mono break-all">{e.target}</span>}
            {e.status && <span className="font-mono">status {e.status}</span>}
          </div>
          {e.tx && (
            <Link
              to="/tx"
              search={{ id: e.tx }}
              className="inline-flex items-center gap-1 font-mono text-[12px] text-route hover:underline"
            >
              {e.tx} <ArrowRight className="size-3" />
            </Link>
          )}
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label="Fechar"
          className="rounded p-1 text-ink-3 hover:bg-paper hover:text-ink"
        >
          <X className="size-4" />
        </button>
      </header>

      <div className="min-h-0 flex-1 space-y-4 overflow-auto px-4 py-3">
        {e.error && (
          <section className="rounded-md border border-fail/30 bg-fail-soft px-3 py-2 font-mono text-[12px] break-words text-fail">
            {e.error}
          </section>
        )}
        {hasPayload(e) && (
          <Section title="Payload" copyText={pretty(e.payload)}>
            <Payload value={e.payload} openDepth={3} full />
          </Section>
        )}
        {Object.keys(fields).length > 0 && (
          <Section title="Campos" copyText={pretty(fields)}>
            <Payload value={fields} openDepth={1} full />
          </Section>
        )}
        <Section title={`Linha crua · ${e.format} · ${e.stream}`} copyText={e.raw}>
          <pre className="max-h-80 overflow-auto rounded bg-paper p-2 font-mono text-[11.5px] break-all whitespace-pre-wrap text-ink-2">
            {e.raw}
          </pre>
        </Section>
      </div>
    </aside>
  )
}

function Section({ title, copyText, children }: { title: string; copyText: string; children: ReactNode }) {
  return (
    <section>
      <div className="mb-1.5 flex items-center justify-between">
        <h3 className="text-[11px] font-semibold tracking-wider text-ink-3 uppercase">{title}</h3>
        <CopyBtn text={copyText} />
      </div>
      {children}
    </section>
  )
}
