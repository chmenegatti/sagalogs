export type EventKind =
  | 'http_in'
  | 'http_out'
  | 'outbox_enqueue'
  | 'outbox_relay'
  | 'job_request'
  | 'job_response'
  | 'step_start'
  | 'state_in'
  | 'step_end'
  | 'publish'
  | 'integration_call'
  | 'error'
  | 'log'
  | 'unknown'

export type Role = 'gateway' | 'orchestrator' | 'worker'
export type Vendor = 'fortinet' | 'paloalto' | 'nsxt' | 'ontap' | 'rubrik' | 'citrix' | 'entraid' | 'vmware' | 'interno' | (string & {})

export interface AppInfo {
  name: string
  label: string
  role: Role
  vendor: Vendor
  /** worker que fecha a saga (label sagalogs.final) */
  final?: boolean
  image?: string
  version?: string
}

export interface SagaEvent {
  id: string
  ts: number
  app: string
  format: 'json' | 'logrus' | 'slog'
  kind: EventKind
  level: string
  msg: string
  action?: string
  resource?: string
  integration?: string
  integrationMethod?: string
  resourceType?: string
  target?: string
  exchange?: string
  routing?: string
  status?: string
  error?: string
  durationMs?: number
  source?: string
  broker?: Record<string, unknown>
  payload?: unknown
  raw: string
  stepId?: string
}

export interface Call {
  ts: number
  type: string
  target: string
  payload?: unknown
  eventId: string
  durationMs?: number
  status?: string
}

export interface Step {
  id: string
  order: number
  app: string
  kind: 'gateway' | 'outbox' | 'orchestrator' | 'worker'
  label: string
  action?: string
  resource?: string
  start: number
  end: number
  durationMs: number
  /** warn: logou erro, mas não publicou ERROR; fail: publicou ERROR (ou HTTP 4xx/5xx no gateway) */
  status: 'ok' | 'warn' | 'fail'
  /** erros logados na etapa, mensagem completa */
  errors?: StepError[]
  publishStatus?: string
  httpStatus?: string
  endpoint?: string
  sagaIndex?: number
  routingIn?: string
  publish?: { exchange: string; routing: string }
  broker?: Record<string, unknown>
  execution?: number
  /** >1 quando o worker ficou em polling (republicou na própria routing key) */
  attempts?: number
  payloadIn?: unknown
  payloadOut?: unknown
  eventIds: string[]
  calls: Call[]
  jobCalls: Call[]
}

export interface StepError {
  ts: number
  eventId: string
  msg: string
  cause?: ErrorCause
}

/** De onde o erro veio (classificado pela API a partir da mensagem). */
export interface ErrorCause {
  kind: 'credential' | 'timeout' | 'concurrency' | 'infra' | 'vendor' | 'ours' | 'retries' | 'other'
  /** fabricante que respondeu; vazio = nosso lado */
  vendor?: string
  /** a mensagem que o fornecedor devolveu, tirada do JSON dentro do erro */
  reply?: string
}

/** ok | fail | recovered (etapa logou erro, mas a saga seguiu e concluiu) | running (sem worker final ainda) */
export type SagaStatus = 'ok' | 'fail' | 'recovered' | 'running'

export interface Edge {
  from: string
  to: string
  waitMs: number
  routing?: string
  exchange?: string
  via: 'outbox' | 'queue' | 'http'
}

export interface Saga {
  transactionId: string
  createdAt?: string
  start: number
  end: number
  durationMs: number
  status: SagaStatus
  /** um worker final processou: a saga terminou */
  finished: boolean
  operation: string
  /** organização do caminho da requisição */
  org?: string
  /** user_id do gateway */
  user?: string
  jobId?: number
  /** ainda havia logs no fim da janela máxima de busca */
  truncated?: boolean
  apps: AppInfo[]
  steps: Step[]
  edges: Edge[]
  events: SagaEvent[]
}

export interface SagaSummary {
  transactionId: string
  operation: string
  status: SagaStatus
  durationMs: number
  start: number
  steps: number
  apps: number
}

/** Ação (POST/PUT/PATCH/DELETE) que chegou ao gateway — /api/requests. */
export interface GatewayRequest {
  transactionId: string
  ts: number
  method: 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  path: string
  operation: string
  resource: string
  org?: string
  user?: string
  /** status_code, quando o gateway loga a resposta */
  status?: string
  durationMs?: number
  /** gravou no outbox: virou saga nos workers */
  saga: boolean
  /** algum worker publicou ERROR */
  sagaFailed?: boolean
}

export interface RequestsPage {
  from: number
  to: number
  requests: GatewayRequest[]
  truncated: boolean
}

/** Transação em que o ID do recurso aparece — /api/resource/{id}. */
export interface ResourceHit {
  transactionId: string
  first: number
  last: number
  /** apps em cujos logs o ID aparece */
  apps: string[]
  lines: number
  /** o ID está no caminho da requisição (e não só em payload) */
  inPath: boolean
  /** ação no gateway; ausente em consultas (GET) e chamadas internas */
  request?: GatewayRequest
}

export interface ResourcePage {
  id: string
  from: number
  to: number
  hits: ResourceHit[]
  truncated: boolean
}

/** Saga sem worker final e sem atividade — /api/stalled. */
export interface StalledSaga {
  transactionId: string
  operation: string
  method: string
  path: string
  start: number
  lastActivity: number
  status?: SagaStatus
  steps?: number
  lastStep?: string
  lastApp?: string
  lastPublish?: string
  error?: string
}

export interface StalledPage {
  from: number
  to: number
  idleMs: number
  checked: number
  stalled: StalledSaga[]
  truncated: boolean
}

/** Desempenho por etapa — /api/perf */
export interface SlowRef {
  transactionId: string
  ms: number
  waitMs: number
  start: number
}
export interface StepPerf {
  key: string
  label: string
  app: string
  kind: Step['kind']
  order: number
  n: number
  p50: number
  p95: number
  max: number
  waitP50: number
  waitP95: number
  fails: number
  warns: number
  attempts: number
  slowest: SlowRef[]
}
export interface PerfReport {
  operation: string
  n: number
  p50: number
  p95: number
  max: number
  fails: number
  steps: StepPerf[]
  days: { day: string; n: number; p50: number; p95: number; fails: number }[]
}
export interface PerfPage {
  from: number
  to: number
  total: number
  sampled: number
  report: PerfReport
}
export interface PerfOpsPage {
  from: number
  to: number
  operations: { operation: string; sagas: number }[]
}

/** Tipos de erro do período — /api/errors */
export interface NameCount {
  name: string
  count: number
}
export interface ErrorSample {
  transactionId: string
  ts: number
  app: string
  operation?: string
  org?: string
  /** a saga falhou (publish ERROR) ou a ação foi recusada pelo gateway */
  failed: boolean
  msg: string
}
export interface ErrorGroup {
  key: string
  /** mensagem sem IDs, números e nomes: o que as ocorrências têm em comum */
  pattern: string
  /** a ocorrência mais recente, completa */
  example: string
  /** algum worker publicou ERROR com esta mensagem */
  published: boolean
  cause: ErrorCause
  lines: number
  sagas: number
  failed: number
  apps: NameCount[]
  operations: NameCount[]
  first: number
  last: number
  hist: number[]
  samples: ErrorSample[]
}
export interface ErrorsPage {
  from: number
  to: number
  lines: number
  groups: ErrorGroup[]
  /** transações distintas com erro e quantas falharam (os grupos se sobrepõem) */
  totals: { txs: number; failed: number }
  truncated: boolean
}

/** Erros novos ou em alta na janela recente — /api/errors/news */
export interface ErrorNews {
  kind: 'new' | 'spike'
  group: ErrorGroup
  baseSagas: number
  /** média de ações afetadas por janela do tamanho da recente, nos 7 dias anteriores */
  basePerWindow: number
}
export interface ErrorNewsPage {
  from: number
  to: number
  baseFrom: number
  groups: number
  news: ErrorNews[]
  truncated: boolean
}

/** Versões (imagens) de cada aplicação no período — /api/versions */
export interface VersionStat {
  app: string
  image: string
  tag: string
  execs: number
  failed: number
  errors: number
  firstSeen: number
  lastSeen: number
  since?: boolean
}
export interface AppVersions {
  app: string
  versions: VersionStat[]
  worse: boolean
}
export interface VersionsPage {
  from: number
  to: number
  apps: AppVersions[]
  partial: boolean
}

/** Uma operação no mapa de falhas: cells = [hora desde start, ações, recusadas (HTTP ≥ 400), sagas que falharam]. */
export interface HeatRow {
  operation: string
  total: number
  failed: number
  cells: [number, number, number, number][]
}

/** /api/heatmap: ações por operação e hora. */
export interface HeatmapPage {
  from: number
  to: number
  /** começo da primeira hora (epoch ms) */
  start: number
  hours: number
  rows: HeatRow[]
  truncated: boolean
  /** a busca não terminou a tempo e segue no servidor: perguntar de novo */
  pending?: boolean
}

/** Soma de ações, falhas e sagas (métricas). */
export interface OpTotals {
  total: number
  failed: number
  saga: number
}

/** Uma operação na tela Métricas. */
export interface OpMetric {
  operation: string
  total: number
  /** recusadas pelo gateway (HTTP ≥ 400) */
  refused: number
  /** recusadas + sagas que falharam */
  failed: number
  sagaFailed: number
  saga: number
  /** o mesmo período no dia anterior (só com período de até 24 h) */
  prev?: OpTotals
  /** [hora desde start, ações, falhas], só horas com ação */
  hours: [number, number, number][]
}

export interface OpsMetricsPage {
  from: number
  to: number
  unplaced: number
  metrics: {
    start: number
    hours: number
    totals: OpTotals
    prev?: OpTotals
    prevFrom?: number
    prevTo?: number
    ops: OpMetric[]
  }
}

/** Tempo total das sagas concluídas de uma operação, do gateway ao worker final (ms). */
export interface OpDuration {
  /** vazio no total */
  operation?: string
  /** sagas medidas */
  n: number
  p50: number
  p95: number
  p99: number
  max: number
  /** sagas por faixa de DUR_BUCKETS (lib/report.ts), para somar dias */
  hist?: number[]
}

export interface SagaDurationsPage {
  from: number
  to: number
  /** sagas concluídas sem a ação de origem no gateway */
  unplaced: number
  durations: { all: OpDuration; ops: OpDuration[] }
}

/** Execuções de um worker que começaram numa hora cheia. */
export interface StepHour {
  /** epoch ms do começo da hora */
  h: number
  n: number
  p95: number
}

export interface WorkerDeploy {
  image: string
  tag: string
  /** epoch ms da primeira linha da imagem */
  ts: number
}

/** Tempo das execuções de um worker no período (do "Setting up worker" à última linha da transação). */
export interface WorkerSteps {
  app: string
  label: string
  n: number
  p50: number
  p95: number
  /** p95 nas últimas 24 h e nos dias antes */
  recentP95: number
  beforeP95: number
  /** recentP95 / beforeP95 - 1; null com poucas execuções */
  trend: number | null
  hours: StepHour[]
  deploys: WorkerDeploy[]
}

export interface StepTimesPage {
  from: number
  to: number
  /** começo do trecho coberto pelo banco local */
  since?: number
  workers: WorkerSteps[]
}

/** Operação, organização ou par das duas com volume de ações fora do normal na última hora. */
export interface VolumeSpike {
  operation?: string
  org?: string
  /** ações na última hora */
  count: number
  /** média por hora nos 7 dias antes */
  perWindow: number
  /** maior hora dos 7 dias antes */
  peak: number
  /** count / perWindow; 0 quando é novo */
  ratio: number
  /** nenhuma ação nos 7 dias antes */
  new?: boolean
  topUser?: string
  topCount?: number
}

export interface VolumePage {
  from: number
  to: number
  baseFrom: number
  spikes: VolumeSpike[]
  partial: boolean
}

/** Execuções, retentativas (entregas repetidas da fila), esgotamentos e sagas com retentativa. */
export interface RetryTotals {
  execs: number
  retries: number
  exhausted: number
  txs: number
}

export interface WorkerRetry extends RetryTotals {
  app: string
  /** [índice da hora, retentativas, esgotamentos] */
  hours: [number, number, number][]
  prev?: RetryTotals
}

export interface RetriesPage {
  from: number
  to: number
  /** o banco local só cobre a partir daqui (o começo do período ficou de fora) */
  since?: number
  metrics: {
    start: number
    hours: number
    totals: RetryTotals
    prev?: RetryTotals
    prevFrom?: number
    prevTo?: number
    workers: WorkerRetry[]
  }
}

/** Sagas abertas (sem worker final) no fim de cada hora. */
export interface FlowPage {
  from: number
  to: number
  /** o banco local só cobre a partir daqui (as sagas abertas antes ficaram de fora) */
  since?: number
  metrics: {
    start: number
    hours: number
    idleMs: number
    lookbackMs: number
    /** [índice da hora, andamento, paradas, paradas com falha] */
    points: [number, number, number, number][]
    sagas: number
    finished: number
    peak: { hour: number; open: number }
    /** andamento, paradas, com falha agora */
    now?: [number, number, number]
  }
}

/** Ações que falharam por causa provável e hora (publish ERROR ou recusadas pelo gateway). */
export interface CausesPage {
  from: number
  to: number
  truncated?: boolean
  causes: {
    start: number
    hours: number
    total: number
    /** dessas, recusadas pelo gateway (não viraram saga) */
    refused: number
    /** na ordem fixa das causas, só as que aparecem */
    kinds: {
      kind: ErrorCause['kind']
      total: number
      /** dessas, recusadas pelo gateway */
      refused: number
      vendors?: { name: string; count: number }[]
      /** [índice da hora, ações, dessas recusadas pelo gateway] */
      hours: [number, number, number][]
    }[]
  }
}
