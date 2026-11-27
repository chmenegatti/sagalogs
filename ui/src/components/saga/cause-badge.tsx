import { CAUSE_HELP, causeText } from '@/lib/cause'
import type { ErrorCause } from '@/lib/saga-types'
import { cn } from '@/lib/utils'

const TONE: Record<ErrorCause['kind'], string> = {
  credential: 'bg-fail-soft text-fail',
  timeout: 'bg-wait-soft text-wait-ink',
  concurrency: 'bg-wait-soft text-wait-ink',
  infra: 'bg-fail-soft text-fail',
  vendor: 'bg-route-soft text-route',
  ours: 'bg-fail-soft text-fail',
  retries: 'bg-paper text-ink-2',
  other: 'bg-paper text-ink-3',
}

/** Causa provável do erro: para quem abrir o chamado. */
export function CauseBadge({ cause, className }: { cause: ErrorCause; className?: string }) {
  return (
    <span
      title={CAUSE_HELP[cause.kind]}
      className={cn('inline-flex items-center rounded-full px-2 py-px text-[12px] font-medium whitespace-nowrap', TONE[cause.kind], className)}
    >
      {causeText(cause)}
    </span>
  )
}

/** "Resposta da Rubrik: ..." quando o erro traz a mensagem do fornecedor num JSON. */
export function VendorReply({ cause }: { cause?: ErrorCause }) {
  if (!cause?.reply) return null
  return (
    <p className="mt-1.5 rounded-md border border-route/30 bg-route-soft/50 px-3 py-2 text-[13px] text-ink">
      <span className="font-medium">Resposta {cause.vendor ? `de ${cause.vendor}` : 'do fornecedor'}:</span> {cause.reply}
    </p>
  )
}
