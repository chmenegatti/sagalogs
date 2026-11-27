import type { QueryClient } from '@tanstack/react-query'
import { Link, Outlet, createRootRouteWithContext } from '@tanstack/react-router'
import { Moon, Sun } from 'lucide-react'
import type { ReactNode } from 'react'
import { ProjectPicker } from '@/components/project-picker'
import { useLive } from '@/lib/api'
import { useContainers } from '@/lib/hooks'
import { useTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'

export const Route = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  component: Root,
})

function Root() {
  useContainers()
  return (
    <div className="flex h-dvh flex-col">
      <header className="z-30 shrink-0 border-b border-rule bg-paper/90 backdrop-blur">
        <div className="flex h-12 items-center gap-6 px-4">
          <Link to="/" className="flex items-center gap-2.5 text-ink">
            <Logo />
            <span className="text-[15px] font-semibold tracking-tight">SagaLogs</span>
          </Link>
          <nav aria-label="Seções" className="flex items-center gap-1 text-[13px]">
            <NavLink to="/">Ao vivo</NavLink>
            <NavLink to="/tx">Transações</NavLink>
          </nav>
          <ProjectPicker />
          <div className="ml-auto flex items-center gap-4">
            <Conn />
            <ThemeToggle />
          </div>
        </div>
      </header>
      <main className="min-h-0 flex-1">
        <Outlet />
      </main>
    </div>
  )
}

function NavLink({ to, children }: { to: '/' | '/tx'; children: ReactNode }) {
  return (
    <Link
      to={to}
      activeOptions={{ exact: true, includeSearch: false }}
      className="rounded-md px-2.5 py-1.5 text-ink-2 hover:text-ink"
      activeProps={{ className: 'bg-sheet text-ink font-medium shadow-[0_0_0_1px_var(--rule)]' }}
    >
      {children}
    </Link>
  )
}

function Conn() {
  const { conn, events } = useLive()
  const label = { live: 'ao vivo', connecting: 'conectando', down: 'sem conexão' }[conn]
  return (
    <span className="flex items-center gap-2 text-[12px] text-ink-2" role="status">
      <span className="relative flex size-2">
        {conn === 'live' && <span className="absolute inset-0 animate-ping rounded-full bg-ok opacity-60" />}
        <span
          className={cn(
            'relative size-2 rounded-full',
            conn === 'live' ? 'bg-ok' : conn === 'down' ? 'bg-fail' : 'bg-wait',
          )}
        />
      </span>
      {label}
      <span className="font-mono text-ink-3">{events.length.toLocaleString('pt-BR')} linhas</span>
    </span>
  )
}

function ThemeToggle() {
  const { theme, toggle } = useTheme()
  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={theme === 'dark' ? 'Tema claro' : 'Tema escuro'}
      className="rounded-md p-1.5 text-ink-2 hover:bg-sheet hover:text-ink"
    >
      {theme === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
    </button>
  )
}

function Logo() {
  return (
    <svg viewBox="0 0 24 24" className="size-5" aria-hidden>
      <rect x="2" y="3" width="20" height="18" rx="4" fill="var(--route)" />
      <path d="M6 9h5M6 12h9M6 15h6" stroke="var(--sheet)" strokeWidth="1.8" strokeLinecap="round" />
      <circle cx="17.5" cy="9" r="1.6" fill="var(--sheet)" />
    </svg>
  )
}
