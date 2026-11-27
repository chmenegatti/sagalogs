import { createRef, useEffect, useMemo, useRef, useState, type RefObject } from 'react'
import { RefreshCw } from 'lucide-react'
import { AnimatedBeam } from '@/components/ui/animated-beam'
import { BorderBeam } from '@/components/ui/border-beam'
import type { Saga, Step } from '@/lib/saga-types'
import { fmtMs } from '@/lib/format'
import { cn } from '@/lib/utils'
import { VENDOR_LABEL, iconFor, shortLabel } from './app-icon'
import type { Replay } from './replay'

/**
 * A saga como linha de metrô: cada etapa é uma estação, na ordem real dos logs.
 * A linha serpenteia (ida e volta) para caber sem rolagem horizontal.
 */
export function MetroMap({
  saga,
  selectedId,
  onSelect,
  replay,
}: {
  saga: Saga
  selectedId: string
  onSelect: (id: string) => void
  replay: Replay
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const cols = useColumns(containerRef)
  const nodeRefs = useMemo(
    () => saga.steps.map(() => createRef<HTMLDivElement>()) as RefObject<HTMLDivElement | null>[],
    [saga],
  )
  const apps = useMemo(() => new Map(saga.apps.map((a) => [a.name, a])), [saga])
  const pos = (i: number) => {
    const row = Math.floor(i / cols)
    const c = i % cols
    return { row, col: row % 2 === 0 ? c : cols - 1 - c }
  }

  return (
    <div className="rounded-lg border border-rule bg-sheet">
      <div
        ref={containerRef}
        className="relative grid gap-x-4 gap-y-12 px-5 pt-8 pb-10 md:px-8"
        style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
      >
        {saga.edges.map((e, i) => {
          const from = pos(i)
          const to = pos(i + 1)
          const traversed = !replay.active || i < replay.stepIndex
          const live = replay.playing && i === replay.stepIndex - 1
          return (
            <AnimatedBeam
              key={`${e.from}-${e.to}-${cols}`}
              containerRef={containerRef}
              fromRef={nodeRefs[i]}
              toRef={nodeRefs[i + 1]}
              reverse={to.col < from.col}
              pathColor="var(--route)"
              pathWidth={4}
              pathOpacity={traversed ? 0.85 : 0.15}
              gradientStartColor={live ? '#ffffff' : 'transparent'}
              gradientStopColor={live ? 'var(--route)' : 'transparent'}
              duration={1.1}
            />
          )
        })}

        {saga.steps.map((s, i) => {
          const { row, col } = pos(i)
          const edgeIn = saga.edges[i - 1]
          const reached = !replay.active || i <= replay.stepIndex
          return (
            <Station
              key={s.id}
              step={s}
              nodeRef={nodeRefs[i]}
              vendor={apps.get(s.app)?.vendor}
              icon={iconFor(s, apps.get(s.app))}
              waitMs={edgeIn?.waitMs ?? 0}
              selected={s.id === selectedId}
              current={replay.active && i === replay.stepIndex}
              dim={!reached}
              onSelect={() => onSelect(s.id)}
              style={{ gridRow: row + 1, gridColumn: col + 1 }}
            />
          )
        })}
      </div>
    </div>
  )
}

function Station({
  step,
  nodeRef,
  icon: Icon,
  vendor,
  waitMs,
  selected,
  current,
  dim,
  onSelect,
  style,
}: {
  step: Step
  nodeRef: RefObject<HTMLDivElement | null>
  icon: React.ComponentType<{ className?: string }>
  vendor?: keyof typeof VENDOR_LABEL
  waitMs: number
  selected: boolean
  current: boolean
  dim: boolean
  onSelect: () => void
  style: React.CSSProperties
}) {
  const fail = step.status === 'fail'
  const warn = step.status === 'warn'
  const slow = step.durationMs >= 1000
  return (
    <button
      type="button"
      onClick={onSelect}
      style={style}
      aria-pressed={selected}
      className={cn(
        'group relative z-10 flex flex-col items-center rounded-lg px-1 pb-2 text-center transition-opacity duration-300',
        dim && 'opacity-35',
      )}
    >
      <div
        ref={nodeRef}
        className={cn(
          'relative flex size-14 items-center justify-center rounded-full border-[3px] bg-sheet transition-[transform,background-color]',
          fail
            ? 'border-fail text-fail'
            : warn
              ? 'border-wait text-wait-ink'
              : step.kind === 'worker'
                ? 'border-route text-route'
                : 'border-ink text-ink',
          selected && (fail ? 'bg-fail-soft' : warn ? 'bg-wait-soft' : 'bg-route-soft'),
          'group-hover:scale-105',
        )}
      >
        <Icon className="size-6" aria-hidden />
        {step.sagaIndex !== undefined && (
          <span className="absolute -top-2 -right-2 flex h-5 min-w-5 items-center justify-center rounded-full bg-ink px-1 font-mono text-[11px] text-sheet">
            {step.sagaIndex}
          </span>
        )}
        {current && <BorderBeam size={36} duration={1.6} colorFrom="#ffffff" colorTo="var(--route)" borderWidth={3} />}
      </div>

      {/* fundo branco: a linha passa por baixo do rótulo, como num mapa de metrô */}
      <span className="mt-2 flex flex-col items-center rounded bg-sheet px-1.5 py-0.5">
        <span className={cn('text-[14px] leading-tight font-medium text-ink', selected && 'underline decoration-route decoration-2 underline-offset-4')}>
          {shortLabel(step)}
        </span>
        <span className="mt-0.5 text-[12px] text-ink-3">
          {step.kind === 'worker'
            ? [vendor && (VENDOR_LABEL[vendor] ?? vendor), [step.action, step.resource].filter(Boolean).join(' ')].filter(Boolean).join(' · ')
            : step.app}
        </span>
        <span className={cn('mt-1 text-[13px] tabular-nums', slow ? 'font-medium text-ink' : 'text-ink-2')}>
          {fmtMs(step.durationMs)}
        </span>
      </span>
      {(step.attempts ?? 0) > 1 && (
        <span
          className="mt-1 flex items-center gap-1 rounded bg-route-soft px-1.5 py-px text-[11px] text-route"
          title="O worker republicou na própria routing key até a condição ser atendida (polling)"
        >
          <RefreshCw className="size-3" aria-hidden />
          {step.attempts!.toLocaleString('pt-BR')} tentativas
        </span>
      )}
      {waitMs >= 100 && (
        <span className="mt-1 rounded bg-wait-soft px-1.5 py-px text-[11px] text-wait-ink">
          esperou {fmtMs(waitMs)}
        </span>
      )}
    </button>
  )
}

function useColumns(ref: RefObject<HTMLDivElement | null>) {
  const [cols, setCols] = useState(4)
  useEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver(([entry]) => {
      const w = entry.contentRect.width
      setCols(w < 420 ? 2 : w < 640 ? 3 : w < 900 ? 4 : 5)
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [ref])
  return cols
}
