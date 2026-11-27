import { Pause, Play, RotateCcw, Square } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Saga } from '@/lib/saga-types'
import { fmtOffset } from '@/lib/format'
import { cn } from '@/lib/utils'

const SPEEDS = [1, 4, 10] as const

export interface Replay {
  /** replay em andamento ou pausado no meio */
  active: boolean
  playing: boolean
  /** ms desde o início da saga */
  t: number
  speed: number
  stepIndex: number
  stepId: string
  play: () => void
  pause: () => void
  stop: () => void
  setSpeed: (s: number) => void
  seek: (t: number) => void
}

export function useReplay(saga: Saga): Replay {
  const [active, setActive] = useState(false)
  const [playing, setPlaying] = useState(false)
  const [t, setT] = useState(0)
  const [speed, setSpeed] = useState<number>(4)
  const raf = useRef(0)

  useEffect(() => {
    if (!playing) return
    let last = performance.now()
    const tick = (now: number) => {
      const dt = (now - last) * speed
      last = now
      setT((prev) => {
        const next = prev + dt
        if (next >= saga.durationMs) {
          setPlaying(false)
          return saga.durationMs
        }
        return next
      })
      raf.current = requestAnimationFrame(tick)
    }
    raf.current = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(raf.current)
  }, [playing, speed, saga.durationMs])

  const stepIndex = useMemo(() => {
    let idx = 0
    saga.steps.forEach((s, i) => {
      if (s.start - saga.start <= t) idx = i
    })
    return idx
  }, [saga, t])

  const play = useCallback(() => {
    setActive(true)
    setT((prev) => (prev >= saga.durationMs ? 0 : prev))
    setPlaying(true)
  }, [saga.durationMs])
  const pause = useCallback(() => setPlaying(false), [])
  const stop = useCallback(() => {
    setPlaying(false)
    setActive(false)
    setT(0)
  }, [])
  const seek = useCallback((v: number) => {
    setActive(true)
    setT(v)
  }, [])

  return {
    active,
    playing,
    t,
    speed,
    stepIndex,
    stepId: saga.steps[stepIndex].id,
    play,
    pause,
    stop,
    setSpeed,
    seek,
  }
}

export function ReplayBar({ replay, saga, className }: { replay: Replay; saga: Saga; className?: string }) {
  const finished = replay.active && !replay.playing && replay.t >= saga.durationMs
  return (
    <div className={cn('flex items-center gap-2', className)}>
      <button
        type="button"
        onClick={replay.playing ? replay.pause : replay.play}
        className="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[13px] font-medium text-sheet hover:bg-ink/90"
      >
        {replay.playing ? (
          <Pause className="size-3.5" aria-hidden />
        ) : finished ? (
          <RotateCcw className="size-3.5" aria-hidden />
        ) : (
          <Play className="size-3.5" aria-hidden />
        )}
        {replay.playing ? 'Pausar' : finished ? 'Repetir' : replay.active ? 'Continuar' : 'Reproduzir saga'}
      </button>
      {replay.active && (
        <>
          <input
            type="range"
            min={0}
            max={saga.durationMs}
            value={replay.t}
            onChange={(e) => replay.seek(Number(e.target.value))}
            aria-label="Posição do replay"
            className="w-28 accent-[var(--route)] md:w-40"
          />
          <span className="w-16 font-mono text-[12px] text-ink-2 tabular-nums">{fmtOffset(replay.t)}</span>
          <button
            type="button"
            onClick={replay.stop}
            className="inline-flex size-8 items-center justify-center rounded-md text-ink-2 hover:bg-muted"
            aria-label="Encerrar replay"
          >
            <Square className="size-3.5" aria-hidden />
          </button>
        </>
      )}
      <div role="group" aria-label="Velocidade" className="flex rounded-md border border-rule bg-sheet p-0.5">
        {SPEEDS.map((s) => (
          <button
            key={s}
            type="button"
            aria-pressed={replay.speed === s}
            onClick={() => replay.setSpeed(s)}
            className={cn(
              'h-6 rounded px-2 text-[12px]',
              replay.speed === s ? 'bg-route-soft font-medium text-route' : 'text-ink-2 hover:text-ink',
            )}
          >
            {s}×
          </button>
        ))}
      </div>
    </div>
  )
}
