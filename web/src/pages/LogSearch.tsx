import { useState, type FormEvent } from 'react'
import { Badge, Button, ContainerLink, Empty, Panel, Segmented } from '../components/ui'
import { api, ApiError } from '../lib/api'
import { dateTime } from '../lib/format'
import type { LogHit } from '../lib/types'
import { useDashboard } from '../store/DashboardProvider'

const WINDOWS = ['15m', '1h', '6h', '24h'] as const
type Window = (typeof WINDOWS)[number]
const SECONDS: Record<Window, number> = { '15m': 900, '1h': 3600, '6h': 21600, '24h': 86400 }

export function LogSearch() {
  const { overview } = useDashboard()
  const [q, setQ] = useState('')
  const [win, setWin] = useState<Window>('1h')
  const [selected, setSelected] = useState<string[]>([])
  const [hits, setHits] = useState<LogHit[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const run = async (e: FormEvent) => {
    e.preventDefault()
    if (!q.trim()) return
    setBusy(true)
    setError('')
    try {
      const params = new URLSearchParams({ q: q.trim(), since: String(Math.floor(Date.now() / 1000) - SECONDS[win]) })
      if (selected.length) params.set('containers', selected.join(','))
      const res = await api.get<{ hits: LogHit[] }>(`/api/logs/search?${params}`)
      setHits(res.hits)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'search failed')
    } finally {
      setBusy(false)
    }
  }

  const toggle = (id: string) => setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]))

  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold tracking-tight">Log search</h1>
      <Panel>
        <form onSubmit={run} className="flex flex-wrap items-center gap-2">
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="regular expression, case-insensitive"
            className="min-w-64 flex-1 rounded-md border border-zinc-300 bg-white px-3 py-1.5 font-mono text-sm outline-none focus:border-zinc-500 dark:border-zinc-700 dark:bg-zinc-950"
          />
          <Segmented value={win} options={WINDOWS} onChange={setWin} />
          <Button type="submit" disabled={busy || !q.trim()}>
            {busy ? 'Searching…' : 'Search'}
          </Button>
        </form>
        <div className="mt-3 flex flex-wrap gap-1.5">
          {overview?.containers
            .filter((c) => c.state === 'running')
            .map((c) => (
              <button
                key={c.id}
                type="button"
                onClick={() => toggle(c.id)}
                className={`rounded-md border px-2 py-0.5 text-xs ${selected.includes(c.id) ? 'border-zinc-900 bg-zinc-900 text-white dark:border-zinc-100 dark:bg-zinc-100 dark:text-zinc-900' : 'border-zinc-300 text-zinc-600 dark:border-zinc-700 dark:text-zinc-300'}`}
              >
                {c.name}
              </button>
            ))}
          {selected.length > 0 && (
            <Button small onClick={() => setSelected([])}>
              all containers
            </Button>
          )}
        </div>
        {error && <div className="mt-2 text-sm text-red-600 dark:text-red-400">{error}</div>}
      </Panel>
      {hits !== null && (
        <Panel title={`${hits.length} matches`}>
          {hits.length === 0 ? (
            <Empty>No lines matched.</Empty>
          ) : (
            <div className="max-h-[600px] overflow-auto font-mono text-[12px] leading-5">
              {hits.map((h, i) => (
                <div key={i} className="flex gap-2 border-b border-zinc-100 py-0.5 last:border-0 dark:border-zinc-800/60">
                  <span className="shrink-0 tabular-nums text-zinc-400">{dateTime(h.ts)}</span>
                  <span className="shrink-0">
                    <ContainerLink id={h.containerId} name={h.containerName} className="text-xs" />
                  </span>
                  {h.level === 'error' && <Badge tone="critical">error</Badge>}
                  {h.level === 'warn' && <Badge tone="warning">warn</Badge>}
                  <span className="whitespace-pre-wrap break-all">{h.text}</span>
                </div>
              ))}
            </div>
          )}
        </Panel>
      )}
    </div>
  )
}
