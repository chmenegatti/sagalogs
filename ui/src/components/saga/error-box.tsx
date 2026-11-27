import { Check, Copy, TriangleAlert } from 'lucide-react'
import { useState } from 'react'
import type { StepError } from '@/lib/saga-types'
import { copyText } from '@/lib/clipboard'
import { fmtClock } from '@/lib/format'
import { cn } from '@/lib/utils'

/** Mensagem de erro completa, em bloco monoespaçado, com copiar. tone wait = a saga se recuperou. */
export function ErrorBox({ errors, tone = 'fail', className }: { errors: StepError[]; tone?: 'fail' | 'wait'; className?: string }) {
  const [copied, setCopied] = useState(false)
  const text = errors.map((e) => e.msg).join('\n\n')
  return (
    <div
      className={cn(
        'rounded-md border text-[12px]',
        tone === 'fail' ? 'border-fail/40 bg-fail-soft' : 'border-wait/50 bg-wait-soft',
        className,
      )}
    >
      <div className="flex items-center gap-2 px-3 pt-2">
        <TriangleAlert className={cn('size-3.5', tone === 'fail' ? 'text-fail' : 'text-wait-ink')} aria-hidden />
        <span className={cn('font-medium', tone === 'fail' ? 'text-fail' : 'text-wait-ink')}>
          {errors.length === 1 ? 'Mensagem de erro' : `${errors.length} mensagens de erro`}
        </span>
        <button
          type="button"
          onClick={async () => {
            if (await copyText(text)) {
              setCopied(true)
              setTimeout(() => setCopied(false), 1500)
            }
          }}
          className="ml-auto inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-ink-2 hover:bg-sheet/60 hover:text-ink"
        >
          {copied ? <Check className="size-3 text-ok" aria-hidden /> : <Copy className="size-3" aria-hidden />}
          {copied ? 'Copiado' : 'Copiar'}
        </button>
      </div>
      <ul className="max-h-64 space-y-2 overflow-auto px-3 pt-1.5 pb-2.5">
        {errors.map((e) => (
          <li key={e.eventId}>
            <span className="text-ink-3 tabular-nums">{fmtClock(e.ts)}</span>
            <pre className="mt-0.5 font-mono text-[12px] leading-relaxed break-words whitespace-pre-wrap text-ink">{e.msg}</pre>
          </li>
        ))}
      </ul>
    </div>
  )
}
