import { ArrowDownToLine, ArrowUpFromLine, Globe, Maximize2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { BorderBeam } from '@/components/ui/border-beam'
import type { Call, Saga, Step } from '@/lib/saga-types'
import { fmtClock, fmtMs, fmtOffset } from '@/lib/format'
import { cn } from '@/lib/utils'
import { ROLE_LABEL, VENDOR_LABEL, iconFor } from './app-icon'
import { ErrorBox } from './error-box'
import { type PayloadTab, PayloadModal, PayloadTabs } from './payload-view'

export function StepPanel({
  saga,
  step,
  replaying,
  reveal,
}: {
  saga: Saga
  step: Step
  replaying: boolean
  /** muda a cada pedido para mostrar a etapa (ex.: link nos eventos): rola até o painel e pisca */
  reveal?: number
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [flash, setFlash] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (reveal === undefined || !el) return
    const smooth = matchMedia('(prefers-reduced-motion: no-preference)').matches ? 'smooth' : 'auto'
    el.scrollTo({ top: 0, behavior: smooth })
    const r = el.getBoundingClientRect()
    // em tela estreita o painel fica abaixo da tabela: traz para a vista
    if (r.top < 64 || r.top > window.innerHeight - 120) el.scrollIntoView({ block: 'start', behavior: smooth })
    setFlash(true)
    const t = setTimeout(() => setFlash(false), 900)
    return () => clearTimeout(t)
  }, [reveal])

  const app = saga.apps.find((a) => a.name === step.app)
  const Icon = iconFor(step, app)
  const idx = saga.steps.indexOf(step)
  const waitBefore = saga.edges[idx - 1]?.waitMs ?? 0
  const hasIn = step.payloadIn !== undefined
  const hasOut = step.payloadOut !== undefined
  const [tab, setTab] = useState<PayloadTab>(hasIn ? 'in' : 'out')
  const [expanded, setExpanded] = useState(false)

  return (
    <div
      ref={ref}
      className={cn(
        'relative scroll-mt-20 overflow-hidden rounded-lg border border-rule bg-sheet transition-shadow duration-300 lg:max-h-[calc(100dvh-6.5rem)] lg:overflow-y-auto',
        flash && 'shadow-[0_0_0_3px_var(--color-route)]',
      )}
    >
      {replaying && <BorderBeam size={120} duration={4} colorFrom="#ffffff" colorTo="var(--route)" borderWidth={2} />}

      <header className="border-b border-rule p-5">
        <div className="flex items-start gap-3">
          <span
            className={cn(
              'flex size-11 shrink-0 items-center justify-center rounded-full border-[3px]',
              step.status === 'fail'
                ? 'border-fail text-fail'
                : step.status === 'warn'
                  ? 'border-wait text-wait-ink'
                  : step.kind === 'worker'
                  ? 'border-route text-route'
                  : 'border-ink text-ink',
            )}
          >
            <Icon className="size-5" aria-hidden />
          </span>
          <div className="min-w-0">
            <p className="text-[12px] text-ink-3">
              Etapa {idx + 1} de {saga.steps.length}
              {step.sagaIndex !== undefined ? ` · índice ${step.sagaIndex} na routing key` : ''}
            </p>
            <h2 className="text-[18px] leading-snug font-semibold text-ink">{step.label}</h2>
            <p className="mt-0.5 text-[13px] text-ink-2">
              {app ? `${ROLE_LABEL[app.role]}${app.role === 'worker' ? ` ${VENDOR_LABEL[app.vendor] ?? app.vendor}` : ''}` : ''}
            </p>
          </div>
        </div>
        <p className="mt-3 truncate font-mono text-[12px] text-ink-2" title={`${app?.image}:${app?.version}`}>
          {step.app}:{app?.version}
        </p>
      </header>

      {step.errors?.length ? (
        <div className="border-b border-rule p-5">
          {step.status === 'warn' && (
            <p className="mb-2 text-[12px] text-wait-ink">
              O worker logou o erro, mas não publicou ERROR: a saga seguiu.
            </p>
          )}
          <ErrorBox errors={step.errors} tone={step.status === 'fail' ? 'fail' : 'wait'} />
        </div>
      ) : null}

      <dl className="grid grid-cols-2 gap-px border-b border-rule bg-rule">
        <Metric label="Início" value={fmtOffset(step.start - saga.start)} hint={fmtClock(step.start)} />
        <Metric label="Duração" value={fmtMs(step.durationMs)} strong />
        <Metric label="Parado antes" value={waitBefore ? fmtMs(waitBefore) : '—'} tone={waitBefore >= 300 ? 'wait' : undefined} />
        <Metric
          label="Resultado"
          value={
            step.status === 'fail'
              ? `Falhou${step.publishStatus ? ` (${step.publishStatus})` : ''}`
              : step.status === 'warn'
                ? `Erro logado, publicou ${step.publishStatus ?? '—'}`
                : (step.publishStatus ?? step.httpStatus ?? 'OK')
          }
          tone={step.status === 'fail' ? 'fail' : step.status === 'warn' ? 'wait' : 'ok'}
        />
      </dl>

      <Section title="Ação">
        <p className="text-[14px] text-ink">
          <span className="font-medium">{step.action ?? '—'}</span> em <span className="font-medium">{step.resource ?? '—'}</span>
        </p>
        <ul className="mt-2 space-y-1.5 text-[13px]">
          {step.endpoint && (
            <Flow icon={Globe} label="Endpoint">
              {step.endpoint}
            </Flow>
          )}
          {step.routingIn && (
            <Flow icon={ArrowDownToLine} label="Recebeu de">
              {step.routingIn}
            </Flow>
          )}
          {step.publish && (
            <Flow icon={ArrowUpFromLine} label="Publicou em">
              {step.publish.exchange} / {step.publish.routing}
            </Flow>
          )}
        </ul>
      </Section>

      {step.calls.length > 0 && (
        <Section title={`Recursos chamados (${step.calls.length})`}>
          <CallBars calls={step.calls} />
        </Section>
      )}

      {step.jobCalls.length > 0 && (
        <Section title="Conversa com o orquestrador">
          <CallBars calls={step.jobCalls} />
        </Section>
      )}

      {(hasIn || hasOut) && (
        <Section
          title="Payload"
          action={
            <button
              type="button"
              onClick={() => setExpanded(true)}
              className="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[12px] text-route hover:bg-route-soft"
              title="Abrir o payload em tela grande"
            >
              <Maximize2 className="size-3.5" aria-hidden />
              Expandir
            </button>
          }
        >
          <PayloadTabs step={step} tab={tab} onTab={setTab} className="max-h-[380px] text-[12px]" />
          <PayloadModal
            step={step}
            title={step.label}
            subtitle={`${step.app}:${app?.version ?? ''}`}
            tab={tab}
            onTab={setTab}
            open={expanded}
            onOpenChange={setExpanded}
          />
        </Section>
      )}

      {step.broker && (
        <Section title="Mensagem no broker">
          <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-[13px]">
            {Object.entries(step.broker).map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="truncate font-mono text-[12px] text-ink-3">{k}</dt>
                <dd className="truncate font-mono text-[12px] text-ink" title={String(v)}>
                  {String(v)}
                </dd>
              </div>
            ))}
          </dl>
        </Section>
      )}

      <Section title={`Linhas de log (${step.eventIds.length})`} last>
        <LogLines saga={saga} step={step} />
      </Section>
    </div>
  )
}

function Metric({ label, value, hint, strong, tone }: { label: string; value: string; hint?: string; strong?: boolean; tone?: 'ok' | 'wait' | 'fail' }) {
  return (
    <div className="bg-sheet px-5 py-3">
      <dt className="text-[12px] text-ink-3">{label}</dt>
      <dd
        className={cn(
          'mt-0.5 text-[16px] tabular-nums',
          strong ? 'font-semibold text-ink' : 'text-ink',
          tone === 'wait' && 'text-wait-ink',
          tone === 'fail' && 'text-fail',
          tone === 'ok' && 'text-ok',
        )}
      >
        {value}
      </dd>
      {hint && <dd className="font-mono text-[11px] text-ink-3">{hint}</dd>}
    </div>
  )
}

function Section({
  title,
  children,
  last,
  action,
}: {
  title: string
  children: React.ReactNode
  last?: boolean
  action?: React.ReactNode
}) {
  return (
    <section className={cn('p-5', !last && 'border-b border-rule')}>
      <div className="mb-2.5 flex items-center justify-between gap-2">
        <h3 className="text-[13px] font-semibold text-ink">{title}</h3>
        {action}
      </div>
      {children}
    </section>
  )
}

function Flow({ icon: I, label, children }: { icon: React.ComponentType<{ className?: string }>; label: string; children: React.ReactNode }) {
  return (
    <li className="flex items-start gap-2">
      <I className="mt-0.5 size-3.5 shrink-0 text-ink-3" aria-hidden />
      <span className="w-20 shrink-0 text-ink-2">{label}</span>
      <span className="min-w-0 font-mono text-[12px] break-all text-ink">{children}</span>
    </li>
  )
}

/** Barras horizontais: quanto tempo cada chamada levou, na ordem em que aconteceu. */
function CallBars({ calls }: { calls: Call[] }) {
  const max = Math.max(1, ...calls.map((c) => c.durationMs ?? 0))
  return (
    <ol className="space-y-1.5">
      {calls.map((c, i) => {
        const d = c.durationMs ?? 0
        return (
          <li key={i} className="grid grid-cols-[minmax(0,1fr)_64px] items-center gap-x-3 text-[12px]">
            <div className="min-w-0">
              <div className="flex items-baseline gap-1.5">
                <span className="truncate font-mono text-ink" title={`${c.type} ${c.target}`}>
                  <span className="text-ink-3">{c.type}</span> {c.target}
                </span>
                {c.status && (
                  <span className={cn('font-mono', c.status.startsWith('2') ? 'text-ok' : 'text-fail')}>{c.status}</span>
                )}
              </div>
              <div className="mt-0.5 h-1.5 rounded-full bg-paper">
                <div
                  className={cn('h-full rounded-full', d >= 500 ? 'bg-wait' : 'bg-route')}
                  style={{ width: `${Math.max(2, (d / max) * 100)}%` }}
                />
              </div>
            </div>
            <span className="text-right text-ink-2 tabular-nums">{fmtMs(d)}</span>
          </li>
        )
      })}
    </ol>
  )
}

function LogLines({ saga, step }: { saga: Saga; step: Step }) {
  const events = useMemo(
    () => step.eventIds.map((id) => saga.events.find((e) => e.id === id)!).filter(Boolean).sort((a, b) => a.ts - b.ts),
    [saga, step],
  )
  return (
    <ol className="space-y-1 font-mono text-[11.5px] leading-snug">
      {events.map((e) => (
        <li key={e.id} className="flex gap-2">
          <span className="shrink-0 text-ink-3">{fmtClock(e.ts).slice(3)}</span>
          <span className={cn('shrink-0 uppercase', e.level === 'debug' ? 'text-ink-3' : e.level === 'error' ? 'text-fail' : 'text-route')}>
            {e.level.slice(0, 4)}
          </span>
          <span className="min-w-0 truncate text-ink" title={e.raw}>
            {e.target ? `${e.kind} ${e.target}` : e.msg}
          </span>
        </li>
      ))}
    </ol>
  )
}

