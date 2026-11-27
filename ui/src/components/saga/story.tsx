import { ArrowDownToLine, ArrowUpFromLine } from 'lucide-react'
import { useMemo } from 'react'
import { Timeline } from '@/components/ui/timeline'
import type { Saga, Step } from '@/lib/saga-types'
import { fmtMs, fmtOffset } from '@/lib/format'
import { cn } from '@/lib/utils'
import { VENDOR_LABEL, iconFor } from './app-icon'

/** Passo a passo: narrativa vertical de cada etapa, com o que entrou, o que chamou e para onde mandou. */
export function Story({ saga, selectedId, onSelect }: { saga: Saga; selectedId: string; onSelect: (id: string) => void }) {
  const apps = useMemo(() => new Map(saga.apps.map((a) => [a.name, a])), [saga])
  const data = saga.steps.map((s, i) => {
    const app = apps.get(s.app)
    const Icon = iconFor(s, app)
    const wait = saga.edges[i - 1]?.waitMs ?? 0
    return {
      id: s.id,
      title: fmtOffset(s.start - saga.start),
      subtitle: wait >= 100 ? `após ${fmtMs(wait)} parada` : undefined,
      content: (
        <button
          type="button"
          onClick={() => onSelect(s.id)}
          className={cn(
            'w-full rounded-lg border bg-sheet p-4 text-left transition-colors hover:border-route/60',
            s.id === selectedId ? 'border-route' : 'border-rule',
          )}
        >
          <div className="flex items-start gap-3">
            <span
              className={cn(
                'flex size-9 shrink-0 items-center justify-center rounded-full border-2',
                s.kind === 'worker' ? 'border-route text-route' : 'border-ink text-ink',
              )}
            >
              <Icon className="size-4" aria-hidden />
            </span>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-baseline gap-x-2">
                <span className="text-[15px] font-medium text-ink">{s.label}</span>
                <span className="text-[13px] text-ink-3">
                  {s.app}
                  {app?.version ? ` ${app.version}` : ''}
                </span>
                <span className="ml-auto text-[13px] text-ink tabular-nums">{fmtMs(s.durationMs)}</span>
              </div>
              <Sentence step={s} vendor={app ? (VENDOR_LABEL[app.vendor] ?? app.vendor) : ''} />
              {s.calls.length > 0 && (
                <ol className="mt-3 flex flex-wrap gap-1.5">
                  {s.calls.map((c, k) => (
                    <li key={k} className="rounded bg-paper px-2 py-0.5 font-mono text-[12px] text-ink-2">
                      {c.type}.{c.target}
                    </li>
                  ))}
                </ol>
              )}
              <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-[12px] text-ink-2">
                {s.routingIn && (
                  <span className="inline-flex items-center gap-1">
                    <ArrowDownToLine className="size-3.5 text-ink-3" aria-hidden />
                    <span className="font-mono">{s.routingIn}</span>
                  </span>
                )}
                {s.publish && (
                  <span className="inline-flex items-center gap-1">
                    <ArrowUpFromLine className="size-3.5 text-ink-3" aria-hidden />
                    <span className="font-mono">{s.publish.routing}</span>
                  </span>
                )}
              </div>
            </div>
          </div>
        </button>
      ),
    }
  })
  return <Timeline data={data} />
}

function Sentence({ step: s, vendor }: { step: Step; vendor: string }) {
  let text: string
  switch (s.kind) {
    case 'gateway':
      text = `Recebeu ${s.endpoint?.split('/').slice(0, 3).join('/') ?? 'a requisição'}… e respondeu ${s.httpStatus ?? ''}. Gravou a mensagem no outbox.`
      break
    case 'outbox':
      text = 'O relay do outbox entregou a mensagem ao orquestrador e marcou como enviada.'
      break
    case 'orchestrator':
      text = `Criou o job e publicou a primeira etapa em ${s.publish?.exchange ?? 'fila'}.`
      break
    default: {
      const calls = s.calls.length
      const job = s.jobCalls.map((c) => c.target.split(' ')[0]).join(', ')
      text = `${vendor}: ${s.action ?? 'executou'} ${s.resource ?? ''}${calls ? `, com ${calls} chamada${calls > 1 ? 's' : ''} a recursos` : ''}${job ? `. Job: ${job}` : ''}. Status ${s.publishStatus ?? 'ok'}.`
    }
  }
  return <p className="mt-1 max-w-[70ch] text-[14px] leading-relaxed text-ink-2">{text}</p>
}
