import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Boxes, Check, ChevronDown } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'

/** Escolhe quais projetos docker compose seguir. Nenhum marcado = todos. */
export function ProjectPicker() {
  const qc = useQueryClient()
  const q = useQuery({ queryKey: ['projects'], queryFn: api.projects, refetchInterval: 5000 })
  const set = useMutation({
    mutationFn: api.setProjects,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['containers'] })
    },
  })
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent ? e.key === 'Escape' : !ref.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', close)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', close)
    }
  }, [open])

  const all = q.data ?? []
  const selected = all.filter((p) => p.selected)
  const everything = all.length > 0 && selected.length === all.length
  const label = everything ? 'todos os projetos' : selected.length === 1 ? selected[0].name : `${selected.length} projetos`

  const toggle = (name: string) => {
    const cur = new Set(selected.map((p) => p.name))
    if (everything) cur.clear()
    if (cur.has(name)) cur.delete(name)
    else cur.add(name)
    // marcar todos = voltar ao modo "todos" (inclui projetos que subirem depois)
    set.mutate(cur.size === all.length ? [] : [...cur])
  }

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        className="flex items-center gap-2 rounded-md border border-rule bg-sheet px-2.5 py-1 text-[12.5px] text-ink hover:border-ink-3"
      >
        <Boxes className="size-3.5 text-ink-3" aria-hidden />
        <span className="max-w-[220px] truncate">{q.data ? label : '…'}</span>
        <ChevronDown className="size-3.5 text-ink-3" aria-hidden />
      </button>
      {open && (
        <div className="absolute top-full left-0 z-40 mt-1.5 w-[340px] rounded-lg border border-rule bg-sheet p-1.5 shadow-lg">
          <p className="px-2 pt-1 pb-2 text-[11.5px] text-ink-3">
            Projetos docker compose nesta máquina. Os papéis de cada app (gateway, orquestrador, worker) são
            aprendidos pelos logs.
          </p>
          <button
            type="button"
            onClick={() => set.mutate([])}
            className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[13px] hover:bg-paper"
          >
            <Tick on={everything} />
            <span className="font-medium">Todos</span>
          </button>
          <div className="my-1 h-px bg-rule" />
          <ul className="max-h-[50dvh] overflow-auto">
            {all.map((p) => (
              <li key={p.name}>
                <button
                  type="button"
                  onClick={() => toggle(p.name)}
                  className="flex w-full items-start gap-2 rounded-md px-2 py-1.5 text-left hover:bg-paper"
                  title={p.workingDir}
                >
                  <Tick on={p.selected && !everything} dim={everything} />
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2 text-[13px] text-ink">
                      <span className="truncate">{p.name}</span>
                      <span
                        className={cn('size-1.5 shrink-0 rounded-full', p.running ? 'bg-ok' : 'bg-ink-3/50')}
                        aria-hidden
                      />
                      <span className="ml-auto shrink-0 font-mono text-[11px] text-ink-3">
                        {p.running}/{p.containers}
                      </span>
                    </span>
                    {p.workingDir && <span className="block truncate font-mono text-[11px] text-ink-3">{p.workingDir}</span>}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

function Tick({ on, dim }: { on: boolean; dim?: boolean }) {
  return (
    <span
      className={cn(
        'mt-0.5 flex size-4 shrink-0 items-center justify-center rounded border',
        on ? 'border-route bg-route text-sheet' : 'border-rule',
        dim && 'border-route/40 bg-route/15',
      )}
    >
      {(on || dim) && <Check className={cn('size-3', dim && 'text-route')} strokeWidth={3} />}
    </span>
  )
}
