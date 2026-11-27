import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { Search } from 'lucide-react'
import { useDeferredValue, useMemo, useState } from 'react'
import { EventsTable } from '@/components/saga/events-table'
import { Lanes } from '@/components/saga/lanes'
import { MetroMap } from '@/components/saga/metro-map'
import { ReplayBar, useReplay } from '@/components/saga/replay'
import { SagaHeader } from '@/components/saga/saga-header'
import { StepPanel } from '@/components/saga/step-panel'
import { Story } from '@/components/saga/story'
import { type TxSummary, api } from '@/lib/api'
import { ago, serviceColor } from '@/lib/meta'
import type { Saga } from '@/lib/saga-types'
import { cn } from '@/lib/utils'

const VIEWS = [
  { id: 'rota', label: 'Rota' },
  { id: 'raias', label: 'Raias por aplicação' },
  { id: 'historia', label: 'Passo a passo' },
  { id: 'eventos', label: 'Eventos' },
] as const
type View = (typeof VIEWS)[number]['id']

export const Route = createFileRoute('/tx')({
  validateSearch: (s: Record<string, unknown>): { id?: string; view?: View; step?: string } => ({
    id: typeof s.id === 'string' && s.id ? s.id : undefined,
    view: VIEWS.some((v) => v.id === s.view) ? (s.view as View) : undefined,
    step: typeof s.step === 'string' ? s.step : undefined,
  }),
  component: TxPage,
})

function TxPage() {
  const { id } = Route.useSearch()
  const [q, setQ] = useState('')
  const dq = useDeferredValue(q.trim().toLowerCase())
  const [reads, setReads] = useState(false)
  const list = useQuery({
    queryKey: ['txs', dq],
    queryFn: () => api.txs(dq),
    refetchInterval: 2000,
    placeholderData: keepPreviousData,
  })

  return (
    <div className="grid h-full min-h-0 grid-cols-[320px_minmax(0,1fr)]">
      <aside className="flex min-h-0 flex-col border-r border-rule">
        <div className="border-b border-rule p-3">
          <label className="relative block">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-ink-3" />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="transaction_id, rota, serviço…"
              className="h-8 w-full rounded-md border border-rule bg-sheet pr-3 pl-8 text-[13px] placeholder:text-ink-3 focus:border-route focus:outline-none"
            />
          </label>
          <label className="mt-2 flex items-center gap-2 text-[12px] text-ink-2">
            <input type="checkbox" checked={reads} onChange={(e) => setReads(e.target.checked)} className="accent-route" />
            mostrar consultas (GET sem saga: {list.data?.filter(isRead).length ?? 0})
          </label>
        </div>
        <ul className="min-h-0 flex-1 overflow-auto">
          {list.data
            ?.filter((t) => reads || t.tx === id || !isRead(t))
            .map((t) => <TxItem key={t.tx} t={t} active={t.tx === id} />)}
          {list.data && !list.data.length && (
            <li className="px-4 py-10 text-center text-[13px] text-ink-3">
              Nenhuma transação ainda.
              <br />
              Elas aparecem quando um log traz <span className="font-mono">transaction_id</span>.
            </li>
          )}
        </ul>
      </aside>
      {id ? (
        <SagaView key={id} id={id} />
      ) : (
        <div className="flex items-center justify-center text-[13px] text-ink-3">Escolha uma transação à esquerda.</div>
      )}
    </div>
  )
}

/** GET que só passou pelo gateway/orquestrador: leitura, não é saga. */
const isRead = (t: TxSummary) => /^GET /.test(t.entry ?? '') && t.received === 0 && !t.failed && !t.errors

function TxItem({ t, active }: { t: TxSummary; active: boolean }) {
  const failed = t.failed > 0 || t.errors > 0
  return (
    <li>
      <Link
        to="/tx"
        search={{ id: t.tx }}
        className={cn(
          'block border-b border-rule/70 px-3 py-2.5 hover:bg-sheet',
          active && 'bg-sheet shadow-[inset_3px_0_0_var(--route)]',
        )}
      >
        <div className="flex items-center gap-2">
          <span className={cn('size-2 shrink-0 rounded-full', failed ? 'bg-fail' : 'bg-ok')} />
          <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-ink">{t.tx}</span>
          <span className="font-mono text-[11px] text-ink-3">{ago(t.last)}</span>
        </div>
        {t.entry && <div className="mt-1 truncate pl-4 font-mono text-[11.5px] text-ink-2">{t.entry}</div>}
        <div className="mt-1.5 flex items-center gap-2 pl-4 text-[11px] text-ink-3">
          <span className="flex -space-x-0.5">
            {t.services.slice(0, 12).map((s) => (
              <span key={s} title={s} className="size-2 rounded-[2px] ring-1 ring-paper" style={{ background: serviceColor(s) }} />
            ))}
          </span>
          <span>{t.services.length} serviços</span>
          <span>·</span>
          <span>{t.received} payloads</span>
          {failed && <span className="text-fail">· {t.failed + t.errors} erros</span>}
        </div>
      </Link>
    </li>
  )
}


function SagaView({ id }: { id: string }) {
  const { view = 'rota', step } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const [paused, setPaused] = useState(false)
  const q = useQuery({
    queryKey: ['saga', id],
    queryFn: () => api.saga(id),
    // saga em andamento: atualiza sozinha (o buffer é local, é barato)
    refetchInterval: (qq) => (!paused && qq.state.data?.status === 'running' ? 2000 : 5000),
    placeholderData: keepPreviousData,
  })
  if (q.isLoading) return <div className="p-6 text-[13px] text-ink-3">montando a saga…</div>
  if (!q.data)
    return (
      <div className="p-6 text-[13px] text-ink-3">
        Não deu para montar a saga desta transação (fora do buffer?).{' '}
        <button type="button" className="text-route" onClick={() => navigate({ search: {} })}>
          voltar
        </button>
      </div>
    )
  return (
    <SagaBody
      saga={q.data}
      view={view}
      step={step}
      live={q.data.status === 'running' ? { paused, updatedAt: q.dataUpdatedAt, onToggle: () => setPaused((p) => !p) } : undefined}
    />
  )
}

function SagaBody({
  saga,
  view,
  step,
  live,
}: {
  saga: Saga
  view: View
  step?: string
  live?: { paused: boolean; updatedAt: number; onToggle: () => void }
}) {
  const navigate = useNavigate({ from: Route.fullPath })
  const replay = useReplay(saga)
  // sem etapa escolhida: abre na que falhou (ou na última), que é o que interessa
  const fallback = useMemo(
    () => (saga.steps.find((s) => s.status === 'fail') ?? saga.steps.find((s) => s.status === 'warn') ?? saga.steps.at(-1))!.id,
    [saga],
  )
  const selectedId = replay.active ? replay.stepId : (step ?? fallback)
  const selected = saga.steps.find((s) => s.id === selectedId) ?? saga.steps[0]
  const select = (sid: string) => {
    if (replay.active) replay.stop()
    navigate({ search: (p) => ({ ...p, step: sid }), replace: true, resetScroll: false })
  }
  const [reveal, setReveal] = useState<{ id: string; n: number } | null>(null)
  const selectAndReveal = (sid: string) => {
    select(sid)
    setReveal((r) => ({ id: sid, n: (r?.n ?? 0) + 1 }))
  }

  return (
    <div className="h-full overflow-auto">
      <div className="mx-auto max-w-[1480px] px-6 pt-6 pb-16">
        <SagaHeader saga={saga} onSelect={select} selectedId={selected.id} live={live} />

        <div className="mt-8 flex flex-wrap items-center gap-3 border-b border-rule">
          <nav aria-label="Visões da saga" className="-mb-px flex gap-1 overflow-x-auto">
            {VIEWS.map((v) => (
              <Link
                key={v.id}
                from={Route.fullPath}
                search={(p) => ({ ...p, view: v.id })}
                replace
                resetScroll={false}
                className={cn(
                  'border-b-2 px-3 py-2.5 text-sm whitespace-nowrap transition-colors',
                  view === v.id ? 'border-route font-medium text-ink' : 'border-transparent text-ink-2 hover:text-ink',
                )}
              >
                {v.label}
              </Link>
            ))}
          </nav>
          <ReplayBar replay={replay} saga={saga} className="mb-2 ml-auto" />
        </div>

        <div className="mt-6 grid gap-6 xl:grid-cols-[minmax(0,1fr)_420px]">
          <section className="min-w-0">
            {view === 'rota' && <MetroMap saga={saga} selectedId={selected.id} onSelect={select} replay={replay} />}
            {view === 'raias' && <Lanes saga={saga} selectedId={selected.id} onSelect={select} replay={replay} />}
            {view === 'historia' && <Story saga={saga} selectedId={selected.id} onSelect={select} />}
            {view === 'eventos' && <EventsTable saga={saga} onSelect={selectAndReveal} selectedId={selected.id} />}
          </section>
          <aside className="xl:sticky xl:top-4 xl:self-start">
            <StepPanel
              key={selected.id}
              saga={saga}
              step={selected}
              replaying={replay.playing}
              reveal={reveal?.id === selected.id ? reveal.n : undefined}
            />
          </aside>
        </div>
      </div>
    </div>
  )
}
