import { useEffect, useMemo, useRef, useState } from 'react'
import type { Saga } from '@/lib/saga-types'
import { fmtMs } from '@/lib/format'
import { ROLE_LABEL, VENDOR_LABEL } from './app-icon'
import type { Replay } from './replay'

const LABEL_W = 190
const LANE_H = 44
const TOP = 30
const PAD_R = 16

/** Uma raia por aplicação, eixo x = tempo real. Setas mostram a mensagem passando de uma app para a próxima. */
export function Lanes({
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
  const wrap = useRef<HTMLDivElement>(null)
  const [w, setW] = useState(900)
  useEffect(() => {
    const el = wrap.current
    if (!el) return
    const ro = new ResizeObserver(([e]) => setW(Math.max(640, e.contentRect.width)))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const lanes = useMemo(() => {
    const order: string[] = []
    for (const s of saga.steps) if (!order.includes(s.app)) order.push(s.app)
    return order.map((name) => saga.apps.find((a) => a.name === name)!)
  }, [saga])
  const laneOf = (app: string) => lanes.findIndex((l) => l.name === app)
  const orchLane = lanes.findIndex((l) => l.role === 'orchestrator')

  const plotW = w - LABEL_W - PAD_R
  const x = (ts: number) => LABEL_W + ((ts - saga.start) / saga.durationMs) * plotW
  const y = (lane: number) => TOP + lane * LANE_H
  const h = TOP + lanes.length * LANE_H + 8

  const ticks = []
  const stepS = saga.durationMs > 15000 ? 2000 : 1000
  for (let t = 0; t <= saga.durationMs; t += stepS) ticks.push(t)

  return (
    <div ref={wrap} className="overflow-x-auto rounded-lg border border-rule bg-sheet">
      <svg width={w} height={h} role="img" aria-label="Raias por aplicação ao longo do tempo" className="block">
        <defs>
          <marker id="arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto">
            <path d="M0,0 L8,4 L0,8 z" fill="var(--route)" />
          </marker>
          <pattern id="hatch" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
            <rect width="6" height="6" fill="var(--sheet)" />
            <line x1="0" y1="0" x2="0" y2="6" stroke="var(--wait)" strokeOpacity="0.35" strokeWidth="3" />
          </pattern>
        </defs>

        {/* eixo */}
        {ticks.map((t) => (
          <g key={t}>
            <line x1={x(saga.start + t)} x2={x(saga.start + t)} y1={TOP - 6} y2={h} stroke="var(--rule)" />
            <text x={x(saga.start + t)} y={TOP - 12} textAnchor="middle" className="fill-ink-3 font-mono text-[11px]">
              {t / 1000}s
            </text>
          </g>
        ))}

        {/* raias */}
        {lanes.map((a, i) => (
          <g key={a.name}>
            <rect x={0} y={y(i)} width={w} height={LANE_H} fill={i % 2 ? 'transparent' : 'var(--stripe)'} />
            <text x={16} y={y(i) + 19} className="fill-ink text-[13px] font-medium">
              {a.name}
            </text>
            <text x={16} y={y(i) + 34} className="fill-ink-3 text-[11px]">
              {ROLE_LABEL[a.role]}
              {a.role === 'worker' ? ` · ${(VENDOR_LABEL[a.vendor] ?? a.vendor)}` : ''}
            </text>
          </g>
        ))}

        {/* chamadas dos workers ao orquestrador (GET/PATCH do job) */}
        {orchLane >= 0 &&
          saga.steps.flatMap((s) =>
            s.jobCalls.map((c, k) => (
              <g key={`${s.id}-j${k}`}>
                <rect
                  x={x(c.ts)}
                  y={y(orchLane) + 14}
                  width={Math.max(2, x(c.ts + (c.durationMs ?? 0)) - x(c.ts))}
                  height={16}
                  rx={3}
                  fill={c.target.startsWith('PATCH') ? 'var(--ok)' : 'var(--ink-3)'}
                  opacity={0.75}
                >
                  <title>{`${s.app} → ${c.target} (${fmtMs(c.durationMs ?? 0)})`}</title>
                </rect>
                <line
                  x1={x(c.ts)}
                  x2={x(c.ts)}
                  y1={y(orchLane) + 30}
                  y2={y(laneOf(s.app)) + 8}
                  stroke="var(--ink-3)"
                  strokeDasharray="2 3"
                  opacity={0.6}
                />
              </g>
            )),
          )}

        {/* setas entre etapas */}
        {saga.edges.map((e) => {
          const a = saga.steps.find((s) => s.id === e.from)!
          const b = saga.steps.find((s) => s.id === e.to)!
          const x1 = x(a.end)
          const y1 = y(laneOf(a.app)) + LANE_H / 2
          const x2 = Math.max(x(b.start), x1 + 6)
          const y2 = y(laneOf(b.app)) + LANE_H / 2
          if (y1 === y2) return null
          const mx = (x1 + x2) / 2
          return (
            <path
              key={`${e.from}-${e.to}`}
              d={`M${x1},${y1} C${mx + 18},${y1} ${mx - 18},${y2} ${x2 - 2},${y2}`}
              fill="none"
              stroke="var(--route)"
              strokeOpacity={0.55}
              strokeWidth={1.5}
              markerEnd="url(#arrow)"
            >
              <title>{`${a.app} → ${b.app}${e.routing ? ` via ${e.routing}` : ''} · esperou ${fmtMs(e.waitMs)}`}</title>
            </path>
          )
        })}

        {/* etapas */}
        {saga.steps.map((s) => {
          const lane = laneOf(s.app)
          const bx = x(s.start)
          const bw = Math.max(5, x(s.end) - bx)
          const sel = s.id === selectedId
          const fill = s.status === 'fail' ? 'var(--fail)' : s.status === 'warn' ? 'var(--wait)' : s.kind === 'worker' ? 'var(--route)' : 'var(--ink-2)'
          return (
            <g
              key={s.id}
              role="button"
              tabIndex={0}
              aria-label={`${s.label}, ${fmtMs(s.durationMs)}`}
              onClick={() => onSelect(s.id)}
              onKeyDown={(ev) => (ev.key === 'Enter' || ev.key === ' ') && onSelect(s.id)}
              className="cursor-pointer outline-none [&:focus-visible>rect:first-child]:stroke-ink"
            >
              <rect
                x={bx}
                y={y(lane) + 10}
                width={bw}
                height={LANE_H - 20}
                rx={4}
                fill={fill}
                stroke={sel ? 'var(--ink)' : 'transparent'}
                strokeWidth={2}
              />
              {s.calls.map((c, k) => (
                <line
                  key={k}
                  x1={x(c.ts)}
                  x2={x(c.ts)}
                  y1={y(lane) + 13}
                  y2={y(lane) + LANE_H - 13}
                  stroke="white"
                  strokeOpacity={0.8}
                />
              ))}
              {bw > 54 && (
                <text x={bx + 6} y={y(lane) + LANE_H / 2 + 4} className="pointer-events-none fill-sheet text-[11px] font-medium">
                  {fmtMs(s.durationMs)}
                </text>
              )}
              <title>{`${s.label} · ${fmtMs(s.durationMs)}`}</title>
            </g>
          )
        })}

        {/* cabeça do replay */}
        {replay.active && (
          <g>
            <line
              x1={x(saga.start + replay.t)}
              x2={x(saga.start + replay.t)}
              y1={TOP - 6}
              y2={h}
              stroke="var(--ink)"
              strokeWidth={1.5}
            />
            <circle cx={x(saga.start + replay.t)} cy={TOP - 6} r={4} fill="var(--ink)" />
          </g>
        )}
      </svg>
      <div className="flex flex-wrap gap-x-5 gap-y-1 border-t border-rule px-4 py-2.5 text-[12px] text-ink-2">
        <span>Traços brancos dentro da barra: chamadas a recursos (API do firewall, banco)</span>
        <span>Cinza na raia do orquestrador: worker lendo o job · verde: worker gravando o job</span>
      </div>
    </div>
  )
}
