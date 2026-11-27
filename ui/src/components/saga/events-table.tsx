import {
  type ColumnDef,
  type SortingState,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { ArrowUpDown, ChevronRight } from 'lucide-react'
import { Fragment, useMemo, useState } from 'react'
import type { EventKind, Saga, SagaEvent } from '@/lib/saga-types'
import { fmtMs, fmtOffset } from '@/lib/format'
import { cn } from '@/lib/utils'
import { JsonTree } from './json-tree'

const KIND_LABEL: Record<EventKind, string> = {
  http_in: 'Requisição',
  http_out: 'Resposta',
  outbox_enqueue: 'Outbox',
  outbox_relay: 'Relay',
  job_request: 'Job (pedido)',
  job_response: 'Job (resposta)',
  step_start: 'Início',
  state_in: 'State entrada',
  step_end: 'Fim',
  publish: 'Publicação',
  integration_call: 'Chamada',
  error: 'Erro',
  log: 'Log',
  unknown: 'Não mapeado',
}

/** Todas as linhas de log da transação, normalizadas. */
export function EventsTable({ saga, onSelect, selectedId }: { saga: Saga; onSelect: (stepId: string) => void; selectedId?: string }) {
  const [sorting, setSorting] = useState<SortingState>([])
  const [filter, setFilter] = useState('')
  const [app, setApp] = useState('')
  const [open, setOpen] = useState<string | null>(null)

  const data = useMemo(() => (app ? saga.events.filter((e) => e.app === app) : saga.events), [saga, app])

  const columns = useMemo<ColumnDef<SagaEvent>[]>(
    () => [
      {
        id: 'expand',
        header: () => <span className="sr-only">Detalhes</span>,
        cell: ({ row }) => (
          <ChevronRight className={cn('size-3.5 text-ink-3 transition-transform', open === row.original.id && 'rotate-90')} aria-hidden />
        ),
      },
      {
        accessorKey: 'ts',
        header: 'Tempo',
        cell: ({ getValue }) => <span className="font-mono text-ink-2">{fmtOffset((getValue() as number) - saga.start)}</span>,
      },
      { accessorKey: 'app', header: 'Aplicação', cell: ({ getValue }) => <span className="text-ink">{getValue() as string}</span> },
      {
        accessorKey: 'kind',
        header: 'Tipo',
        cell: ({ getValue }) => {
          const k = getValue() as EventKind
          return (
            <span
              className={cn(
                'rounded px-1.5 py-0.5 text-[11.5px]',
                k === 'error' || k === 'unknown'
                  ? 'bg-fail-soft text-fail'
                  : k === 'integration_call'
                    ? 'bg-route-soft text-route'
                    : k === 'publish'
                      ? 'bg-ok-soft text-ok'
                      : 'bg-paper text-ink-2',
              )}
            >
              {KIND_LABEL[k]}
            </span>
          )
        },
      },
      {
        id: 'what',
        header: 'O que aconteceu',
        accessorFn: (e) => `${e.target ?? ''} ${e.msg} ${e.integration ?? ''} ${e.integrationMethod ?? ''}`,
        cell: ({ row }) => {
          const e = row.original
          return (
            <span className="block max-w-[46ch] truncate font-mono text-[12px] text-ink" title={e.target ?? e.msg}>
              {e.target ?? e.msg}
            </span>
          )
        },
      },
      {
        accessorKey: 'durationMs',
        header: 'Duração',
        cell: ({ getValue }) => {
          const v = getValue() as number | undefined
          return <span className="text-ink-2 tabular-nums">{v !== undefined ? fmtMs(v) : ''}</span>
        },
      },
      {
        accessorKey: 'format',
        header: 'Formato',
        cell: ({ getValue }) => <span className="text-ink-3">{getValue() as string}</span>,
      },
    ],
    [saga.start, open],
  )

  const table = useReactTable({
    data,
    columns,
    state: { sorting, globalFilter: filter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
  })

  const apps = [...new Set(saga.events.map((e) => e.app))]

  return (
    <div className="rounded-lg border border-rule bg-sheet">
      <div className="flex flex-wrap items-center gap-3 border-b border-rule p-3">
        <label className="sr-only" htmlFor="ev-filter">
          Filtrar eventos
        </label>
        <input
          id="ev-filter"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Filtrar por texto (ex.: Address-Create, PATCH, routing)"
          className="h-8 w-full max-w-sm rounded-md border border-rule bg-paper px-2.5 text-[13px] outline-none focus:border-route"
        />
        <label className="sr-only" htmlFor="ev-app">
          Aplicação
        </label>
        <select
          id="ev-app"
          value={app}
          onChange={(e) => setApp(e.target.value)}
          className="h-8 rounded-md border border-rule bg-paper px-2 text-[13px] text-ink outline-none focus:border-route"
        >
          <option value="">Todas as aplicações</option>
          {apps.map((a) => (
            <option key={a} value={a}>
              {a}
            </option>
          ))}
        </select>
        <span className="ml-auto text-[12px] text-ink-3">
          {table.getRowModel().rows.length} de {saga.events.length} linhas
        </span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-[13px]">
          <thead className="text-[12px] text-ink-3">
            {table.getHeaderGroups().map((hg) => (
              <tr key={hg.id} className="border-b border-rule">
                {hg.headers.map((h) => (
                  <th key={h.id} className="px-3 py-2 font-medium whitespace-nowrap">
                    {h.column.getCanSort() && h.id !== 'expand' ? (
                      <button type="button" onClick={h.column.getToggleSortingHandler()} className="inline-flex items-center gap-1 hover:text-ink">
                        {flexRender(h.column.columnDef.header, h.getContext())}
                        <ArrowUpDown className="size-3" aria-hidden />
                      </button>
                    ) : (
                      flexRender(h.column.columnDef.header, h.getContext())
                    )}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {table.getRowModel().rows.map((row) => {
              const e = row.original
              const isOpen = open === e.id
              return (
                <Fragment key={row.id}>
                  <tr
                    onClick={() => setOpen(isOpen ? null : e.id)}
                    className={cn(
                      'cursor-pointer border-b border-rule/70 hover:bg-paper',
                      isOpen && 'bg-paper',
                      // linhas da etapa aberta no painel
                      e.stepId && e.stepId === selectedId && 'shadow-[inset_3px_0_0_var(--color-route)]',
                    )}
                  >
                    {row.getVisibleCells().map((c) => (
                      <td key={c.id} className="px-3 py-1.5 whitespace-nowrap">
                        {flexRender(c.column.columnDef.cell, c.getContext())}
                      </td>
                    ))}
                  </tr>
                  {isOpen && (
                    <tr className="border-b border-rule bg-paper/60">
                      <td colSpan={columns.length} className="px-4 py-3">
                        <div className="grid gap-4 xl:grid-cols-2">
                          <div>
                            <p className="mb-1 text-[12px] font-medium text-ink-2">Linha original</p>
                            <pre className="max-h-64 overflow-auto rounded border border-rule bg-sheet p-2 font-mono text-[11.5px] whitespace-pre-wrap break-all text-ink">
                              {e.raw}
                            </pre>
                            {e.source && <p className="mt-1 font-mono text-[11px] text-ink-3">{e.source}</p>}
                          </div>
                          <div>
                            <p className="mb-1 text-[12px] font-medium text-ink-2">Payload extraído</p>
                            <div className="max-h-64 overflow-auto rounded border border-rule bg-sheet p-2 font-mono text-[12px]">
                              {e.payload !== undefined ? <JsonTree value={e.payload} /> : <span className="text-ink-3">sem payload nesta linha</span>}
                            </div>
                            {e.stepId && (
                              <button
                                type="button"
                                onClick={(ev) => {
                                  ev.stopPropagation()
                                  onSelect(e.stepId!)
                                }}
                                className="mt-2 text-[13px] text-route underline underline-offset-4"
                              >
                                {e.stepId === selectedId ? 'Mostrar a etapa no painel (já aberta)' : 'Abrir etapa no painel'}
                              </button>
                            )}
                          </div>
                        </div>
                      </td>
                    </tr>
                  )}
                </Fragment>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
