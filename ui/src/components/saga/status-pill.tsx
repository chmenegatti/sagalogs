import { Check, CirclePause, LoaderCircle, RotateCcw, X } from 'lucide-react'
import type { SagaStatus } from '@/lib/saga-types'
import { cn } from '@/lib/utils'

type Look = SagaStatus | 'stalled'
const LOOK: Record<Look, { label: string; cls: string; Icon: typeof Check }> = {
  ok: { label: 'Concluída', cls: 'bg-ok-soft text-ok', Icon: Check },
  fail: { label: 'Falhou', cls: 'bg-fail-soft text-fail', Icon: X },
  recovered: { label: 'Falhou e se recuperou', cls: 'bg-wait-soft text-wait-ink', Icon: RotateCcw },
  running: { label: 'Em andamento', cls: 'bg-route-soft text-route', Icon: LoaderCircle },
  stalled: { label: 'Parada', cls: 'bg-wait-soft text-wait-ink', Icon: CirclePause },
}

/** stalled: running sem atividade há mais de 15 min (lib/stall). */
export function StatusPill({ status, stalled, className }: { status: SagaStatus; stalled?: boolean; className?: string }) {
  const { label, cls, Icon } = LOOK[stalled ? 'stalled' : status] ?? LOOK.fail
  return (
    <span className={cn('inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[12px] font-medium', cls, className)}>
      <Icon className={cn('size-3.5', status === 'running' && !stalled && 'animate-spin [animation-duration:2s]')} aria-hidden />
      {label}
    </span>
  )
}
