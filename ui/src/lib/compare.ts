import type { Step } from './saga-types'

/** Etapa "equivalente" entre duas execuções: mesmo worker (ou mesmo tipo, para gateway/outbox/orquestrador). */
export const stepKey = (s: Step) => (s.kind === 'worker' ? `w:${s.app}` : s.kind)

export interface AlignedRow {
  key: string
  a?: Step
  b?: Step
  /** status, publish ou HTTP diferentes, ou a etapa só existe de um lado */
  differs: boolean
}

const sameOutcome = (a: Step, b: Step) =>
  a.status === b.status && (a.publishStatus ?? '') === (b.publishStatus ?? '') && (a.httpStatus ?? '') === (b.httpStatus ?? '')

/** Alinha as etapas das duas execuções pela maior subsequência comum de etapas (LCS),
 *  então etapas repetidas (polling, paloalto-rule ×3) e etapas extras de um lado ficam no lugar certo. */
export function alignSteps(A: Step[], B: Step[]): AlignedRow[] {
  const ka = A.map(stepKey)
  const kb = B.map(stepKey)
  const L = Array.from({ length: A.length + 1 }, () => new Array<number>(B.length + 1).fill(0))
  for (let i = A.length - 1; i >= 0; i--)
    for (let j = B.length - 1; j >= 0; j--) L[i][j] = ka[i] === kb[j] ? L[i + 1][j + 1] + 1 : Math.max(L[i + 1][j], L[i][j + 1])

  const rows: AlignedRow[] = []
  let i = 0
  let j = 0
  while (i < A.length || j < B.length) {
    if (i < A.length && j < B.length && ka[i] === kb[j]) {
      rows.push({ key: ka[i], a: A[i], b: B[j], differs: !sameOutcome(A[i], B[j]) })
      i++
      j++
    } else if (j >= B.length || (i < A.length && L[i + 1][j] >= L[i][j + 1])) {
      rows.push({ key: ka[i], a: A[i++], differs: true })
    } else {
      rows.push({ key: kb[j], b: B[j++], differs: true })
    }
  }
  return rows
}

/** Primeira etapa em que as execuções divergiram (-1 se seguiram o mesmo caminho com o mesmo resultado). */
export const firstDivergence = (rows: AlignedRow[]) => rows.findIndex((r) => r.differs)

type Flat = Record<string, string>
export function flatten(v: unknown, prefix = '', out: Flat = {}): Flat {
  if (v !== null && typeof v === 'object') {
    for (const [k, x] of Object.entries(v)) flatten(x, prefix ? `${prefix}.${k}` : k, out)
  } else out[prefix || '(valor)'] = JSON.stringify(v)
  return out
}

/** Datas de auditoria: mudam sempre, nunca explicam nada. */
export const AUDIT_FIELDS = /(UpdatedAt|CreatedAt)$/

/** Entre duas execuções também mudam sempre os IDs gerados e os horários. */
export const VOLATILE_FIELDS = /(^|\.)(id|uuid)$|(Id|ID|_id|Uuid|UUID|At|_at|Time|time|Timestamp|timestamp|Date)$/

/** Valores gerados (UUID, data ISO) mudam entre execuções mesmo em campos de nome neutro (ex.: JobStatus = id do job). */
const GENERATED_VALUE = /^"([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|\d{4}-\d\d-\d\dT[^"]*)"$/i

export const isVolatile = (r: DiffRow) =>
  VOLATILE_FIELDS.test(r.k) || ((r.a === undefined || GENERATED_VALUE.test(r.a)) && (r.b === undefined || GENERATED_VALUE.test(r.b)))

export interface DiffRow {
  k: string
  a?: string
  b?: string
}

/** Campos diferentes entre dois JSONs. hidden = quantos foram ignorados pelo filtro. */
export function diffJson(
  before: unknown,
  after: unknown,
  ignore?: RegExp | ((r: DiffRow) => boolean),
): { rows: DiffRow[]; hidden: number } {
  const a = flatten(before)
  const b = flatten(after)
  const keys = [...new Set([...Object.keys(a), ...Object.keys(b)])].sort()
  const changed = keys.map((k) => ({ k, a: a[k], b: b[k] })).filter((r) => r.a !== r.b)
  const skip = ignore instanceof RegExp ? (r: DiffRow) => ignore.test(r.k) : ignore
  const rows = skip ? changed.filter((r) => !skip(r)) : changed
  return { rows, hidden: changed.length - rows.length }
}
