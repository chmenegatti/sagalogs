import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { NumberTicker } from '@/components/ui/number-ticker'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { copyText } from '@/lib/clipboard'
import type { Saga } from '@/lib/saga-types'
import { fmtClock, fmtDate, fmtMs } from '@/lib/format'
import { cn } from '@/lib/utils'
import { mainCause } from '@/lib/cause'
import { CauseBadge, VendorReply } from './cause-badge'
import { ErrorBox } from './error-box'
import { StatusPill } from './status-pill'
import { isStalled } from '@/lib/stall'

export function SagaHeader({
  saga,
  selectedId,
  onSelect,
  live,
}: {
  saga: Saga
  selectedId: string
  onSelect: (id: string) => void
  /** saga em andamento sendo atualizada sozinha */
  live?: { paused: boolean; updatedAt: number; onToggle: () => void }
}) {
  const busy = saga.steps.reduce((acc, s) => acc + s.durationMs, 0)
  const waiting = Math.max(0, saga.durationMs - unionMs(saga))
  return (
    <section>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <h1 className="text-[26px] leading-tight font-semibold tracking-tight text-ink">{saga.operation}</h1>
        <StatusPill status={saga.status} stalled={isStalled(saga)} />
        {live && <LiveControl {...live} />}
      </div>
      <div className="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-[13px] text-ink-2">
        <CopyTx tx={saga.transactionId} />
        <span>
          {fmtDate(saga.start)}, {fmtClock(saga.start)} a {fmtClock(saga.end)}
        </span>
        {saga.jobId && <span>Job {saga.jobId}</span>}
      </div>
      {(saga.user || saga.org) && (
        <div className="mt-1.5 flex flex-wrap items-center gap-x-6 gap-y-1 text-[13px]">
          {saga.user && <IdField label="Usuário" id={saga.user} filter={{ user: saga.user }} at={saga.start} />}
          {saga.org && <IdField label="Organização" id={saga.org} filter={{ org: saga.org }} at={saga.start} />}
        </div>
      )}
      {saga.truncated && (
        <p className="mt-3 max-w-[70ch] rounded border border-wait/40 bg-wait-soft px-3 py-2 text-[13px] text-wait-ink">
          A saga ainda tinha logs no fim da janela máxima de busca. As últimas etapas podem estar faltando.
        </p>
      )}

      <FailureSummary saga={saga} selectedId={selectedId} onSelect={onSelect} />
      <StallNotice saga={saga} onSelect={onSelect} />

      <dl className="mt-6 grid grid-cols-2 gap-x-8 gap-y-4 sm:grid-cols-3 lg:grid-cols-5">
        <Stat label="Duração ponta a ponta" {...timeStat(saga.durationMs)} big />
        <Stat label="Etapas" value={saga.steps.length} />
        <Stat label="Aplicações" value={saga.apps.length} />
        <Stat label="Linhas de log" value={saga.events.length} />
        <Stat label="Parada entre etapas" {...timeStat(waiting)} tone="wait" />
      </dl>

      <DurationStrip saga={saga} selectedId={selectedId} onSelect={onSelect} />
      <p className="sr-only">Tempo somado em processamento: {fmtMs(busy)}</p>
    </section>
  )
}

/** Onde a saga falhou, com a mensagem completa. Se ela se recuperou, diz isso com todas as letras. */
function FailureSummary({ saga, selectedId, onSelect }: { saga: Saga; selectedId: string; onSelect: (id: string) => void }) {
  const recovered = saga.status !== 'fail'
  // falha = etapa que publicou ERROR (ou HTTP 4xx/5xx); senão, os alertas (erro só logado)
  const failed = saga.steps.filter((s) => (recovered ? s.status === 'warn' : s.status === 'fail'))
  if (saga.status === 'ok' || failed.length === 0) return null
  const last = saga.steps.indexOf(failed[failed.length - 1])
  const after = saga.steps.length - 1 - last
  return (
    <section
      aria-label={recovered ? 'Falha recuperada' : 'Falha'}
      className={cn('mt-4 max-w-[920px] rounded-lg border p-4', recovered ? 'border-wait/50 bg-sheet' : 'border-fail/40 bg-sheet')}
    >
      <h2 className={cn('text-[15px] font-semibold', recovered ? 'text-wait-ink' : 'text-fail')}>
        {recovered
          ? failed.length === 1
            ? 'Uma etapa logou erro, mas a saga se recuperou'
            : `${failed.length} etapas logaram erro, mas a saga se recuperou`
          : `A saga falhou em ${failed[failed.length - 1].label}`}
      </h2>
      <p className="mt-1 text-[13px] text-ink-2">
        {recovered
          ? `Nenhum worker publicou ERROR, então a saga seguiu${after > 0 ? `: ${after === 1 ? 'a etapa seguinte concluiu' : `as ${after} etapas seguintes concluíram`}` : ''}. Vale olhar o erro, mas a saga não falhou.`
          : 'Um worker publicou ERROR: a saga falhou. Veja a mensagem e a etapa abaixo.'}
      </p>
      <ul className="mt-3 space-y-3">
        {failed.slice(0, 3).map((st) => (
          <li key={st.id}>
            <div className="mb-1.5 flex flex-wrap items-center gap-x-2 text-[13px]">
              <span className="font-medium text-ink">{st.label}</span>
              <span className="font-mono text-[12px] text-ink-3">{st.app}</span>
              {mainCause(st.errors) && <CauseBadge cause={mainCause(st.errors)!} />}
              {st.id !== selectedId && (
                <button type="button" onClick={() => onSelect(st.id)} className="text-[12px] text-route underline-offset-4 hover:underline">
                  Ver etapa
                </button>
              )}
            </div>
            {st.errors?.length ? (
              <>
                <ErrorBox errors={st.errors} tone={recovered ? 'wait' : 'fail'} />
                <VendorReply cause={mainCause(st.errors)} />
              </>
            ) : (
              <p className="text-[12px] text-ink-3">A etapa foi marcada como falha, mas não logou mensagem de erro.</p>
            )}
          </li>
        ))}
      </ul>
      {failed.length > 3 && <p className="mt-2 text-[12px] text-ink-3">E mais {failed.length - 3} etapas com falha (veja na rota).</p>}
    </section>
  )
}

/** Indicador de atualização automática, com pausar/retomar. */
function LiveControl({ paused, updatedAt, onToggle }: { paused: boolean; updatedAt: number; onToggle: () => void }) {
  return (
    <span className="inline-flex items-center gap-2 text-[12px] text-ink-2">
      <span className="relative flex size-2" aria-hidden>
        {!paused && <span className="absolute inline-flex size-full animate-ping rounded-full bg-route opacity-60" />}
        <span className={cn('relative inline-flex size-2 rounded-full', paused ? 'bg-ink-3' : 'bg-route')} />
      </span>
      {paused ? 'Atualização pausada' : `Ao vivo, atualizado às ${fmtClock(updatedAt, false)}`}
      <button
        type="button"
        onClick={onToggle}
        className="rounded border border-rule px-1.5 py-0.5 text-ink-2 hover:bg-paper hover:text-ink"
      >
        {paused ? 'Retomar' : 'Pausar'}
      </button>
    </span>
  )
}

/** Saga sem worker final: em andamento ou parada. Diz onde parou e há quanto tempo. */
function StallNotice({ saga, onSelect }: { saga: Saga; onSelect: (id: string) => void }) {
  if (saga.status !== 'running') return null
  const stalled = isStalled(saga)
  const last = saga.steps[saga.steps.length - 1]
  const idle = Date.now() - saga.end
  return (
    <section
      aria-label={stalled ? 'Saga parada' : 'Saga em andamento'}
      className={cn('mt-4 max-w-[920px] rounded-lg border bg-sheet p-4', stalled ? 'border-wait/50' : 'border-route/40')}
    >
      <h2 className={cn('text-[15px] font-semibold', stalled ? 'text-wait-ink' : 'text-route')}>
        {stalled ? `Parou depois de ${last.label}, há ${fmtMs(idle)}` : 'Saga em andamento'}
      </h2>
      <p className="mt-1 text-[13px] text-ink-2">
        {stalled ? 'Nenhum worker final processou esta saga, e não há logs novos. ' : `Última atividade há ${fmtMs(idle)}. `}
        {last.publishStatus === 'IN_PROGRESS'
          ? stalled
            ? `O ${last.app} publicou IN_PROGRESS${last.publish ? ` em ${last.publish.routing}` : ''} e ninguém continuou depois.`
            : `O ${last.app} publicou IN_PROGRESS: ainda está trabalhando (costuma voltar a rodar sozinho, em polling).`
          : last.publish
            ? stalled
              ? `O ${last.app} publicou em ${last.publish.routing}, mas nenhum worker consumiu.`
              : `O ${last.app} publicou em ${last.publish.routing}; a próxima etapa ainda não começou.`
            : `A última etapa registrada é ${last.app}.`}{' '}
        {stalled && 'Se precisar, reprocesse pelo Outbox do edge-board.'}
      </p>
      <button type="button" onClick={() => onSelect(last.id)} className="mt-2 text-[12px] text-route underline-offset-4 hover:underline">
        Ver última etapa
      </button>
    </section>
  )
}

/** tempo coberto por pelo menos uma etapa */
function unionMs(saga: Saga) {
  const iv = saga.steps.map((s) => [s.start, s.end] as const).sort((a, b) => a[0] - b[0])
  let total = 0
  let [cs, ce] = iv[0]
  for (const [s, e] of iv.slice(1)) {
    if (s > ce) {
      total += ce - cs
      ;[cs, ce] = [s, e]
    } else ce = Math.max(ce, e)
  }
  return total + (ce - cs)
}

// sagas com polling passam de horas: troca a unidade para continuar legível
function timeStat(ms: number) {
  if (ms >= 2 * 3600_000) return { value: ms / 3600_000, decimals: 1, unit: 'h' }
  if (ms >= 120_000) return { value: ms / 60_000, decimals: 1, unit: 'min' }
  return { value: ms / 1000, decimals: 1, unit: 's' }
}

function Stat({
  label,
  value,
  decimals = 0,
  unit,
  big,
  tone,
}: {
  label: string
  value: number
  decimals?: number
  unit?: string
  big?: boolean
  tone?: 'wait'
}) {
  return (
    <div>
      <dt className="text-[13px] text-ink-2">{label}</dt>
      <dd className={cn('mt-0.5 flex items-baseline gap-1 font-medium text-ink', big ? 'text-[32px]' : 'text-[24px]')}>
        <NumberTicker value={value} decimalPlaces={decimals} className={cn('tracking-tight', tone === 'wait' ? 'text-wait' : 'text-ink')} />
        {unit && <span className="text-[15px] font-normal text-ink-2">{unit}</span>}
      </dd>
    </div>
  )
}


function IdField({ label, id }: { label: string; id: string; filter?: unknown; at?: number }) {
  const [copied, setCopied] = useState(false)
  return (
    <span className="inline-flex flex-wrap items-center gap-x-2">
      <span className="text-ink-2">{label}</span>
      <button
        type="button"
        onClick={async () => {
          if (await copyText(id)) {
            setCopied(true)
            setTimeout(() => setCopied(false), 1400)
          }
        }}
        className="group inline-flex items-center gap-1.5 rounded font-mono text-[13px] text-ink hover:text-route"
        title="Copiar o ID"
      >
        {id}
        {copied ? <Check className="size-3.5 text-ok" aria-hidden /> : <Copy className="size-3.5 text-ink-3 group-hover:text-route" aria-hidden />}
      </button>
    </span>
  )
}

function CopyTx({ tx }: { tx: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      type="button"
      onClick={async () => {
        if (await copyText(tx)) {
          setCopied(true)
          setTimeout(() => setCopied(false), 1400)
        }
      }}
      className="group inline-flex items-center gap-1.5 rounded font-mono text-[13px] text-ink hover:text-route"
      aria-label="Copiar transaction_id"
    >
      {tx}
      {copied ? (
        <Check className="size-3.5 text-ok" aria-hidden />
      ) : (
        <Copy className="size-3.5 text-ink-3 group-hover:text-route" aria-hidden />
      )}
      <span aria-live="polite" className="sr-only">
        {copied ? 'Copiado' : ''}
      </span>
    </button>
  )
}

/** Faixa de tempo: cada etapa no seu intervalo real; o fundo hachurado é tempo parado. */
function DurationStrip({ saga, selectedId, onSelect }: { saga: Saga; selectedId: string; onSelect: (id: string) => void }) {
  const pct = (ms: number) => (ms / saga.durationMs) * 100
  return (
    <div className="mt-6">
      <div className="hatch-wait relative h-7 rounded-md border border-rule bg-sheet">
        {saga.steps.map((s) => {
          const left = pct(s.start - saga.start)
          const width = Math.max(pct(s.durationMs), 0.35)
          const selected = s.id === selectedId
          return (
            <Tooltip key={s.id}>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  onClick={() => onSelect(s.id)}
                  aria-label={`${s.label}: ${fmtMs(s.durationMs)}`}
                  className={cn(
                    'absolute top-1 bottom-1 rounded-[3px] transition-[box-shadow]',
                    s.kind === 'worker' ? 'bg-route' : 'bg-ink-2',
                    s.status === 'fail' && 'bg-fail',
                    s.status === 'warn' && 'bg-wait',
                    selected && 'ring-2 ring-ink ring-offset-1',
                  )}
                  style={{ left: `${left}%`, width: `${width}%` }}
                />
              </TooltipTrigger>
              <TooltipContent>
                <span className="font-medium">{s.label}</span> · {fmtMs(s.durationMs)}
              </TooltipContent>
            </Tooltip>
          )
        })}
      </div>
      <div className="mt-1.5 flex flex-wrap items-center gap-x-5 gap-y-1 text-[12px] text-ink-2">
        <Legend className="bg-ink-2">Gateway e orquestrador</Legend>
        <Legend className="bg-route">Worker processando</Legend>
        <Legend className="hatch-wait border border-rule">Parada entre etapas (fila, espera do job)</Legend>
        <span className="ml-auto font-mono">0 — {fmtMs(saga.durationMs)}</span>
      </div>
    </div>
  )
}

function Legend({ className, children }: { className: string; children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className={cn('inline-block size-2.5 rounded-[2px]', className)} aria-hidden />
      {children}
    </span>
  )
}
