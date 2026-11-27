import { ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { cn } from '@/lib/utils'

/** Árvore JSON recolhível. openDepth: níveis abertos de início. */
export function JsonTree({
  value,
  depth = 0,
  name,
  openDepth = 2,
  full = false,
}: {
  value: unknown
  depth?: number
  name?: string
  openDepth?: number
  full?: boolean
}) {
  const [open, setOpen] = useState(depth < openDepth)
  const isObj = value !== null && typeof value === 'object'

  if (!isObj) {
    return (
      <div className="flex gap-1.5 py-px pl-4">
        {name !== undefined && <span className="shrink-0 text-ink-2">{name}:</span>}
        <Scalar v={value} full={full} />
      </div>
    )
  }

  const entries = Array.isArray(value) ? value.map((v, i) => [String(i), v] as const) : Object.entries(value)
  const brackets = Array.isArray(value) ? ['[', ']'] : ['{', '}']
  if (!entries.length)
    return (
      <div className="flex gap-1.5 py-px pl-4">
        {name !== undefined && <span className="text-ink-2">{name}:</span>}
        <span className="text-ink-3">{brackets.join('')}</span>
      </div>
    )
  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-expanded={open}
        className="flex items-center gap-0.5 rounded py-px text-left hover:bg-paper"
      >
        <ChevronRight className={cn('size-3.5 text-ink-3 transition-transform', open && 'rotate-90')} aria-hidden />
        {name !== undefined && <span className="text-ink-2">{name}:</span>}
        <span className="ml-1 text-ink-3">
          {brackets[0]}
          {!open && `${entries.length} ${Array.isArray(value) ? 'itens' : 'campos'}${brackets[1]}`}
        </span>
      </button>
      {open && (
        <div className="ml-[7px] border-l border-rule pl-1">
          {entries.map(([k, v]) => (
            <JsonTree key={k} name={k} value={v} depth={depth + 1} openDepth={openDepth} full={full} />
          ))}
          <span className="pl-3 text-ink-3">{brackets[1]}</span>
        </div>
      )}
    </div>
  )
}

function Scalar({ v, full }: { v: unknown; full: boolean }) {
  if (v === null || v === undefined) return <span className="text-ink-3">null</span>
  if (typeof v === 'number') return <span className="text-route">{v}</span>
  if (typeof v === 'boolean') return <span className="text-json-bool">{String(v)}</span>
  const s = String(v)
  return <span className="break-all text-ok">"{!full && s.length > 400 ? s.slice(0, 400) + '…' : s}"</span>
}

/** Payload: árvore se for objeto, texto se for string solta. */
export function Payload({ value, openDepth = 2, full }: { value: unknown; openDepth?: number; full?: boolean }) {
  if (value !== null && typeof value === 'object')
    return (
      <div className="font-mono text-[12px] leading-[1.55]">
        <JsonTree value={value} openDepth={openDepth} full={full} />
      </div>
    )
  return <pre className="font-mono text-[12px] break-all whitespace-pre-wrap text-ink-2">{String(value)}</pre>
}
