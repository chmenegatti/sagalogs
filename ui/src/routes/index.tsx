import { createFileRoute } from '@tanstack/react-router'
import { ArrowDown, Braces, Eraser, Pause, Play, Search } from 'lucide-react'
import { type ReactNode, memo, useCallback, useDeferredValue, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { KindPill, ServiceTag, StepTag, Toggle, TxChip } from '@/components/bits'
import { EventDetail } from '@/components/event-detail'
import { Payload } from '@/components/json-tree'
import { type Kind, type Level, type LogEvent, api, resetLive, useLive } from '@/lib/api'
import { KIND, KIND_ORDER, hasPayload, serviceColor, time } from '@/lib/meta'
import { cn } from '@/lib/utils'
import { useContainers } from '@/lib/hooks'

export const Route = createFileRoute('/')({ component: Live })

interface Filters {
  hidden: string[] // containers escondidos
  kinds: Kind[] // vazio = todos
  levels: Level[] // vazio = todos
  noise: boolean // mostrar healthcheck & cia
  onlyTx: boolean
  expand: boolean // payloads abertos na lista
}

const DEFAULTS: Filters = { hidden: [], kinds: [], levels: [], noise: false, onlyTx: false, expand: true }
const KEY = 'sagalogs-filters'

function useFilters() {
  const [f, setF] = useState<Filters>(() => {
    try {
      return { ...DEFAULTS, ...JSON.parse(localStorage.getItem(KEY) ?? '{}') }
    } catch {
      return DEFAULTS
    }
  })
  useEffect(() => {
    try {
      localStorage.setItem(KEY, JSON.stringify(f))
    } catch {
      /* sem storage */
    }
  }, [f])
  const set = useCallback((p: Partial<Filters>) => setF((o) => ({ ...o, ...p })), [])
  return [f, set] as const
}

const PAGE = 400

function Live() {
  const { events } = useLive()
  const containers = useContainers()
  const [f, set] = useFilters()
  const [q, setQ] = useState('')
  const dq = useDeferredValue(q.trim().toLowerCase())
  const [paused, setPaused] = useState<LogEvent[] | null>(null)
  const [selected, setSelected] = useState<LogEvent | null>(null)
  const [limit, setLimit] = useState(PAGE)

  const source = paused ?? events

  const counts = useMemo(() => {
    const m = new Map<string, number>()
    for (const e of events) m.set(e.container, (m.get(e.container) ?? 0) + 1)
    return m
  }, [events])

  const filtered = useMemo(() => {
    const hidden = new Set(f.hidden)
    const kinds = new Set(f.kinds)
    const levels = new Set(f.levels)
    return source.filter(
      (e) =>
        !hidden.has(e.container) &&
        (f.noise || !e.noise) &&
        (!kinds.size || kinds.has(e.kind)) &&
        (!levels.size || levels.has(e.level)) &&
        (!f.onlyTx || e.tx) &&
        (!dq || e.raw.toLowerCase().includes(dq)),
    )
  }, [source, f, dq])

  const visible = filtered.length > limit ? filtered.slice(filtered.length - limit) : filtered

  // ── rolagem: gruda no fim enquanto o usuário estiver lá ──
  const scroller = useRef<HTMLDivElement>(null)
  const stick = useRef(true)
  const [atBottom, setAtBottom] = useState(true)
  const seenLen = useRef(filtered.length)
  const [unseen, setUnseen] = useState(0)

  useLayoutEffect(() => {
    const el = scroller.current
    if (!el) return
    if (stick.current) {
      el.scrollTop = el.scrollHeight
      seenLen.current = filtered.length
      setUnseen(0)
    } else {
      setUnseen(Math.max(0, filtered.length - seenLen.current))
    }
  }, [filtered.length, visible])

  const onScroll = () => {
    const el = scroller.current!
    const bottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
    stick.current = bottom
    setAtBottom(bottom)
    if (bottom) {
      seenLen.current = filtered.length
      setUnseen(0)
    }
  }
  const toBottom = () => {
    stick.current = true
    const el = scroller.current
    if (el) el.scrollTop = el.scrollHeight
    setAtBottom(true)
  }

  const services = useMemo(() => {
    const names = new Set(containers.map((c) => c.name))
    for (const k of counts.keys()) names.add(k)
    return [...names].sort()
  }, [containers, counts])

  const toggleIn = <T,>(list: T[], v: T) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v])

  return (
    <div
      className={cn(
        'grid h-full min-h-0',
        selected ? 'grid-cols-[248px_minmax(0,1fr)_minmax(380px,560px)]' : 'grid-cols-[248px_minmax(0,1fr)]',
      )}
    >
      {/* ─── filtros ─── */}
      <aside className="flex min-h-0 flex-col gap-5 overflow-auto border-r border-rule px-3 py-4">
        <Group
          title="Containers"
          action={
            <button
              type="button"
              className="text-[11px] text-ink-3 hover:text-route"
              onClick={() => set({ hidden: f.hidden.length ? [] : services })}
            >
              {f.hidden.length ? 'todos' : 'nenhum'}
            </button>
          }
        >
          <ul className="space-y-0.5">
            {services.map((name) => {
              const c = containers.find((x) => x.name === name)
              const on = !f.hidden.includes(name)
              return (
                <li key={name}>
                  <button
                    type="button"
                    onClick={(ev) =>
                      set({
                        // alt/meta: só este
                        hidden: ev.altKey || ev.metaKey ? services.filter((s) => s !== name) : toggleIn(f.hidden, name),
                      })
                    }
                    title={`${c?.status ?? 'fora do compose'} — alt+clique: só este`}
                    className={cn(
                      'flex w-full items-center gap-2 rounded-md px-2 py-1 text-left transition-colors hover:bg-sheet',
                      !on && 'opacity-40',
                    )}
                  >
                    <span
                      className="size-2.5 shrink-0 rounded-[3px]"
                      style={{ background: on ? serviceColor(name) : 'transparent', boxShadow: `inset 0 0 0 1.5px ${serviceColor(name)}` }}
                    />
                    <span className="min-w-0 flex-1 truncate font-mono text-[12px]">{name}</span>
                    {c && c.state !== 'running' && <span className="text-[10px] text-fail uppercase">{c.state}</span>}
                    <span className="font-mono text-[11px] text-ink-3">{counts.get(name) ?? 0}</span>
                  </button>
                </li>
              )
            })}
            {!services.length && <li className="px-2 text-[12px] text-ink-3">nenhum container do projeto</li>}
          </ul>
        </Group>

        <Group title="Tipo">
          <div className="flex flex-wrap gap-1">
            {KIND_ORDER.map((k) => {
              const on = f.kinds.includes(k)
              const Icon = KIND[k].icon
              return (
                <button
                  key={k}
                  type="button"
                  aria-pressed={on}
                  onClick={() => set({ kinds: toggleIn(f.kinds, k) })}
                  className={cn(
                    'inline-flex h-6 items-center gap-1 rounded border px-1.5 text-[11px]',
                    on ? 'border-route bg-route-soft text-route' : 'border-rule bg-sheet text-ink-2 hover:text-ink',
                  )}
                >
                  <Icon className="size-3" />
                  {KIND[k].label}
                </button>
              )
            })}
          </div>
        </Group>

        <Group title="Nível">
          <div className="flex flex-wrap gap-1">
            {(['debug', 'info', 'warn', 'error'] as Level[]).map((l) => {
              const on = f.levels.includes(l)
              return (
                <button
                  key={l}
                  type="button"
                  aria-pressed={on}
                  onClick={() => set({ levels: toggleIn(f.levels, l) })}
                  className={cn(
                    'h-6 rounded border px-2 font-mono text-[11px]',
                    on ? 'border-route bg-route-soft text-route' : 'border-rule bg-sheet text-ink-2 hover:text-ink',
                  )}
                >
                  {l}
                </button>
              )
            })}
          </div>
        </Group>

        <Group title="Exibição">
          <div className="flex flex-col items-start gap-1.5">
            <Toggle on={f.expand} onChange={(v) => set({ expand: v })}>
              <Braces className="size-3.5" /> payloads abertos
            </Toggle>
            <Toggle on={f.onlyTx} onChange={(v) => set({ onlyTx: v })}>
              só com transaction_id
            </Toggle>
            <Toggle on={f.noise} onChange={(v) => set({ noise: v })}>
              mostrar healthcheck
            </Toggle>
          </div>
        </Group>
      </aside>

      {/* ─── stream ─── */}
      <section className="relative flex min-h-0 flex-col">
        <div className="flex shrink-0 items-center gap-2 border-b border-rule bg-paper px-3 py-2">
          <label className="relative flex-1">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-ink-3" />
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder="Buscar em tudo (transaction_id, campo, texto)…"
              className="h-8 w-full rounded-md border border-rule bg-sheet pr-3 pl-8 text-[13px] text-ink placeholder:text-ink-3 focus:border-route focus:outline-none"
            />
          </label>
          <span className="font-mono text-[12px] whitespace-nowrap text-ink-3">
            {filtered.length.toLocaleString('pt-BR')} / {source.length.toLocaleString('pt-BR')}
          </span>
          <button
            type="button"
            onClick={() => setPaused(paused ? null : events)}
            className={cn(
              'inline-flex h-8 items-center gap-1.5 rounded-md border px-2.5 text-[12px]',
              paused ? 'border-wait bg-wait-soft text-wait-ink' : 'border-rule bg-sheet text-ink-2 hover:text-ink',
            )}
          >
            {paused ? <Play className="size-3.5" /> : <Pause className="size-3.5" />}
            {paused ? `retomar (+${events.length - paused.length})` : 'pausar'}
          </button>
          <button
            type="button"
            onClick={async () => {
              await api.clear()
              resetLive()
              setPaused(null)
              setSelected(null)
            }}
            title="Limpa o buffer (no servidor também)"
            className="inline-flex h-8 items-center gap-1.5 rounded-md border border-rule bg-sheet px-2.5 text-[12px] text-ink-2 hover:text-ink"
          >
            <Eraser className="size-3.5" /> limpar
          </button>
        </div>

        <div ref={scroller} onScroll={onScroll} className="min-h-0 flex-1 overflow-auto bg-sheet/40">
          {filtered.length > limit && (
            <button
              type="button"
              onClick={() => setLimit((l) => l + PAGE)}
              className="w-full border-b border-rule py-2 text-[12px] text-route hover:bg-sheet"
            >
              mostrar {Math.min(PAGE, filtered.length - limit)} anteriores ({filtered.length - limit} ocultas)
            </button>
          )}
          {visible.map((e) => (
            <Row
              key={e.seq}
              e={e}
              expand={f.expand}
              selected={selected?.seq === e.seq}
              onSelect={setSelected}
            />
          ))}
          {!visible.length && <Empty hasAny={source.length > 0} />}
        </div>

        {!atBottom && (
          <button
            type="button"
            onClick={toBottom}
            className="absolute bottom-4 left-1/2 inline-flex -translate-x-1/2 items-center gap-1.5 rounded-full bg-ink px-3.5 py-1.5 text-[12px] font-medium text-sheet shadow-lg"
          >
            <ArrowDown className="size-3.5" />
            {unseen > 0 ? `${unseen} novas` : 'ir para o fim'}
          </button>
        )}
      </section>

      {selected && <EventDetail e={selected} onClose={() => setSelected(null)} />}
    </div>
  )
}

function Group({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between px-1">
        <h2 className="text-[11px] font-semibold tracking-wider text-ink-3 uppercase">{title}</h2>
        {action}
      </div>
      {children}
    </div>
  )
}

function Empty({ hasAny }: { hasAny: boolean }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-1 py-20 text-center">
      <p className="text-[14px] text-ink-2">{hasAny ? 'Nada casa com os filtros.' : 'Aguardando logs…'}</p>
      <p className="text-[12px] text-ink-3">
        {hasAny ? 'Ajuste containers, tipo ou busca.' : 'As linhas aparecem aqui assim que os containers escreverem.'}
      </p>
    </div>
  )
}

const Row = memo(function Row({
  e,
  expand,
  selected,
  onSelect,
}: {
  e: LogEvent
  expand: boolean
  selected: boolean
  onSelect: (e: LogEvent) => void
}) {
  const payloadCard = expand && (e.kind === 'received' || e.kind === 'failed') && hasPayload(e)
  return (
    <div
      onClick={() => onSelect(e)}
      className={cn(
        'cursor-pointer border-b border-rule/60 px-3 py-[5px] text-[12.5px] [content-visibility:auto] [contain-intrinsic-size:auto_28px]',
        'hover:bg-sheet',
        selected && 'bg-route-soft/60 hover:bg-route-soft/60',
        e.level === 'error' && !selected && 'bg-fail-soft/40',
        e.kind === 'received' && !selected && 'bg-sheet',
      )}
      style={{ boxShadow: `inset 3px 0 0 ${e.level === 'error' ? 'var(--fail)' : e.level === 'warn' ? 'var(--wait)' : e.kind === 'received' ? serviceColor(e.container) : 'transparent'}` }}
    >
      <div className="flex min-w-0 items-center gap-3">
        <span className="w-[88px] shrink-0 font-mono text-[11.5px] text-ink-3">{time(e.ts)}</span>
        <ServiceTag name={e.container} className="w-[168px] shrink-0" />
        <span className="w-[86px] shrink-0">
          <KindPill kind={e.kind} />
        </span>
        <span className={cn('min-w-0 flex-1 truncate', e.kind === 'log' ? 'text-ink-2' : 'text-ink', e.kind === 'received' && 'font-medium')}>
          {e.msg}
          {e.target && e.target !== e.msg && <span className="ml-2 font-mono text-[11.5px] text-ink-3">{e.target}</span>}
          {e.error && <span className="ml-2 text-fail">{e.error}</span>}
        </span>
        <span className="flex shrink-0 items-center gap-1.5">
          {e.status && e.kind !== 'done' && (
            <span className={cn('font-mono text-[11px]', /^[45]|fail/.test(e.status) ? 'text-fail' : 'text-ink-3')}>{e.status}</span>
          )}
          {e.action && <span className="rounded bg-paper px-1.5 font-mono text-[11px] text-ink-2">{e.action}</span>}
          <StepTag e={e} />
          {!payloadCard && hasPayload(e) && <Braces className="size-3.5 text-ink-3" aria-label="tem payload" />}
          {e.tx && <TxChip tx={e.tx} />}
        </span>
      </div>
      {payloadCard && (
        <div
          className="mt-1.5 mb-1 ml-[100px] max-h-72 overflow-auto rounded-md border border-rule bg-paper/70 px-2 py-1.5"
          onClick={(ev) => ev.stopPropagation()}
        >
          <div className="mb-1 flex items-center gap-2 text-[11px] text-ink-3">
            <span className="font-semibold tracking-wider uppercase">payload recebido</span>
            {e.resource && <span className="font-mono">resource={e.resource}</span>}
            {e.saga && <span className="font-mono">saga={e.saga}</span>}
          </div>
          <Payload value={e.payload} openDepth={1} />
        </div>
      )}
    </div>
  )
})
