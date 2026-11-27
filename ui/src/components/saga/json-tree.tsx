import { ChevronRight, Lock } from 'lucide-react'
import { useState } from 'react'
import { AUDIT_FIELDS, type DiffRow, diffJson } from '@/lib/compare'
import { cn } from '@/lib/utils'

/** Árvore JSON recolhível. Strings não-JSON aparecem como texto.
 *  openDepth: níveis abertos de início; full: não corta strings longas (modal). */
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
        {name !== undefined && <span className="text-ink-2">{name}:</span>}
        <Scalar v={value} full={full} />
      </div>
    )
  }

  const entries = Array.isArray(value) ? value.map((v, i) => [String(i), v] as const) : Object.entries(value)
  const brackets = Array.isArray(value) ? ['[', ']'] : ['{', '}']
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
  // mascarado pela API (saga.Masked): mostra cadeado; a marca #abc123 diz se o valor mudou
  if (s.startsWith('••••••'))
    return (
      <span className="inline-flex items-center gap-1 text-ink-3" title="Valor sensível ocultado. A marca muda quando o valor muda.">
        <Lock className="size-3" aria-hidden />
        oculto<span className="text-ink-3/70">{s.slice(6)}</span>
      </span>
    )
  return <span className="break-all text-ok">"{!full && s.length > 400 ? s.slice(0, 400) + '…' : s}"</span>
}

/** Diferença campo a campo entre dois JSONs: − antes / + depois.
 *  ignore: campos fora do diff (padrão: datas de auditoria). */
export function JsonDiff({
  before,
  after,
  ignore = AUDIT_FIELDS,
  empty = 'Nenhum campo mudou entre a entrada e a saída desta etapa.',
  labels,
}: {
  before: unknown
  after: unknown
  ignore?: RegExp | ((r: DiffRow) => boolean)
  empty?: string
  /** prefixos no lugar de −/+ (ex.: A/B na comparação) */
  labels?: [string, string]
}) {
  const { rows } = diffJson(before, after, ignore)
  if (!rows.length) return <p className="py-2 text-[13px] text-ink-2">{empty}</p>
  return (
    <ul className="space-y-1.5">
      {rows.map((r) => (
        <li key={r.k} className="rounded bg-paper px-2 py-1.5">
          <div className="break-all text-ink-2">{r.k}</div>
          <div className="break-all text-fail">
            {labels?.[0] ?? '−'} {r.a ?? <span className="text-ink-3">(ausente)</span>}
          </div>
          <div className="break-all text-ok">
            {labels?.[1] ?? '+'} {r.b ?? <span className="text-ink-3">(ausente)</span>}
          </div>
        </li>
      ))}
    </ul>
  )
}
