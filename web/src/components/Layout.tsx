import { useState, type ReactNode } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import { api } from '../lib/api'
import { bytes, relative } from '../lib/format'
import type { Theme } from '../lib/theme'
import { useDashboard } from '../store/DashboardProvider'
import { Dot, type Tone } from './ui'
import { AlertBanner } from './AlertBanner'

const NAV = [
  { to: '/', label: 'Overview', end: true },
  { to: '/errors', label: 'Errors' },
  { to: '/disks', label: 'Disks' },
  { to: '/updates', label: 'Updates' },
  { to: '/alerts', label: 'Alerts' },
  { to: '/search', label: 'Log search' },
]

export function Layout({ children, theme, toggleTheme }: { children: ReactNode; theme: Theme; toggleTheme: () => void }) {
  const { overview, connected, errorCount, setAuthenticated, now } = useDashboard()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)

  const logout = async () => {
    try {
      await api.post('/api/auth/logout')
    } finally {
      setAuthenticated(false)
      navigate('/login')
    }
  }

  const sources = overview?.sources ?? {}
  const firing = overview?.alerts.filter((a) => !a.ackedAt).length ?? 0

  return (
    <div className="flex min-h-full flex-col">
      <header className="sticky top-0 z-20 border-b border-zinc-200 bg-white/90 backdrop-blur dark:border-zinc-800 dark:bg-zinc-950/90">
        <div className="mx-auto flex max-w-[1600px] items-center gap-2 px-3 py-2 sm:gap-3 sm:px-4">
          <button type="button" className="rounded-md p-1.5 hover:bg-zinc-100 md:hidden dark:hover:bg-zinc-800" onClick={() => setOpen((o) => !o)} aria-label="Menu">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round">
              <path d="M3 5h14M3 10h14M3 15h14" />
            </svg>
          </button>
          <NavLink to="/" className="flex items-center gap-2 font-semibold tracking-tight">
            <span className="inline-flex h-6 w-6 items-center justify-center rounded-md bg-zinc-900 text-[11px] text-white dark:bg-zinc-100 dark:text-zinc-900">HD</span>
            <span className="hidden sm:inline">{overview?.host.hostname || 'Homelab'}</span>
          </NavLink>
          <nav className="ml-4 hidden items-center gap-1 md:flex">
            {NAV.map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                end={n.end}
                className={({ isActive }) =>
                  `rounded-md px-2.5 py-1 text-sm ${isActive ? 'bg-zinc-100 font-medium text-zinc-900 dark:bg-zinc-800 dark:text-zinc-100' : 'text-zinc-600 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100'}`
                }
              >
                {n.label}
                {n.to === '/errors' && errorCount > 0 && <span className="ml-1.5 rounded-full bg-red-500/15 px-1.5 text-[10px] font-semibold text-red-600 dark:text-red-300">{errorCount}</span>}
                {n.to === '/alerts' && firing > 0 && <span className="ml-1.5 rounded-full bg-red-500 px-1.5 text-[10px] font-semibold text-white">{firing}</span>}
              </NavLink>
            ))}
          </nav>
          <div className="ml-auto flex min-w-0 shrink items-center gap-2 text-xs text-zinc-500 sm:gap-3">
            <div className="hidden items-center gap-2 lg:flex">
              {(['docker', 'portainer', 'host', 'smart', 'registry'] as const).map((name) => {
                const s = sources[name]
                const tone: Tone = !s ? 'neutral' : s.ok ? 'good' : 'critical'
                const title = !s ? `${name}: not polled yet` : s.ok ? `${name}: ok, ${relative(s.lastOk, now)}` : `${name}: ${s.lastError}`
                return (
                  <span key={name} className="flex items-center gap-1" title={title}>
                    <Dot tone={tone} />
                    <span className="hidden xl:inline">{name}</span>
                  </span>
                )
              })}
            </div>
            <span className="flex items-center gap-1" title={connected ? 'Live updates connected' : 'Reconnecting…'}>
              <Dot tone={connected ? 'good' : 'warning'} pulse={!connected} />
              <span className="hidden sm:inline">{connected ? 'live' : 'reconnecting'}</span>
            </span>
            {overview && <span className="hidden xl:inline" title="SQLite size on disk">db {bytes(overview.dbBytes, 0)}</span>}
            <button type="button" onClick={toggleTheme} className="rounded-md border border-zinc-300 px-2 py-1 hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800" title="Toggle theme">
              {theme === 'dark' ? '☾' : '☀'}
            </button>
            <button type="button" onClick={logout} title="Sign out" className="rounded-md border border-zinc-300 px-2 py-1 hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800">
              <span className="hidden sm:inline">Sign out</span>
              <span className="sm:hidden">⏻</span>
            </button>
          </div>
        </div>
        {open && (
          <nav className="flex flex-col border-t border-zinc-200 px-2 py-1 md:hidden dark:border-zinc-800">
            {NAV.map((n) => (
              <NavLink key={n.to} to={n.to} end={n.end} onClick={() => setOpen(false)} className={({ isActive }) => `rounded-md px-3 py-2 text-sm ${isActive ? 'bg-zinc-100 font-medium dark:bg-zinc-800' : 'text-zinc-600 dark:text-zinc-300'}`}>
                {n.label}
              </NavLink>
            ))}
          </nav>
        )}
      </header>
      <AlertBanner />
      <main className="mx-auto w-full max-w-[1600px] flex-1 px-4 py-4">{children}</main>
      <footer className="px-4 py-3 text-center text-[11px] text-zinc-400">
        {overview ? `dashboard ${overview.version} · portainer ${overview.portainer.version} · docker ${overview.host.dockerVersion} · up ${relative(overview.startedAt, now).replace(' ago', '')}` : ''}
      </footer>
    </div>
  )
}
