import { useEffect, useMemo, useState } from 'react'
import { Badge, Button, ContainerLink, Empty, Panel, Spinner, type Tone } from '../components/ui'
import { api } from '../lib/api'
import { dateTime, relative } from '../lib/format'
import type { LogErrorRecord } from '../lib/types'
import { useDashboard } from '../store/DashboardProvider'

const kindTone: Record<string, Tone> = { exit: 'critical', oom: 'critical', health: 'serious', log: 'warning' }

export function Errors() {
  const { now, errorCount, resetErrorCount } = useDashboard()
  const [records, setRecords] = useState<LogErrorRecord[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})

  const load = async () => {
    setLoading(true)
    try {
      setRecords(await api.get<LogErrorRecord[]>('/api/errors'))
      resetErrorCount()
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const groups = useMemo(() => {
    const map = new Map<string, { id: string; name: string; items: LogErrorRecord[] }>()
    for (const r of records ?? []) {
      const g = map.get(r.containerId) ?? { id: r.containerId, name: r.containerName, items: [] }
      g.items.push(r)
      map.set(r.containerId, g)
    }
    return [...map.values()].sort((a, b) => b.items[0].ts - a.items[0].ts)
  }, [records])

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        <h1 className="text-xl font-semibold tracking-tight">Error feed</h1>
        <span className="text-sm text-zinc-500">last 24 hours · log pattern matches, non-zero exits, OOM kills, failing health checks</span>
        <span className="ml-auto">
          <Button small onClick={() => void load()} disabled={loading}>
            {errorCount > 0 ? `Refresh (${errorCount} new)` : 'Refresh'}
          </Button>
        </span>
      </div>
      {records === null ? (
        <div className="flex justify-center p-12">
          <Spinner />
        </div>
      ) : groups.length === 0 ? (
        <Empty>Nothing matched in the last 24 hours. Quiet is good.</Empty>
      ) : (
        groups.map((g) => {
          const open = expanded[g.id] ?? true
          return (
            <Panel
              key={g.id}
              title={
                <span className="flex items-center gap-2">
                  <ContainerLink id={g.id} name={g.name} />
                  <Badge tone="neutral">{g.items.length}</Badge>
                  <span className="text-xs font-normal text-zinc-500">latest {relative(g.items[0].ts, now)}</span>
                </span>
              }
              action={
                <Button small onClick={() => setExpanded((e) => ({ ...e, [g.id]: !open }))}>
                  {open ? 'Collapse' : 'Expand'}
                </Button>
              }
            >
              {open && (
                <div className="max-h-96 overflow-auto font-mono text-[12px] leading-5">
                  {g.items.slice(0, 200).map((r) => (
                    <div key={r.id} className="flex gap-2 border-b border-zinc-100 py-0.5 last:border-0 dark:border-zinc-800/60">
                      <span className="shrink-0 tabular-nums text-zinc-400">{dateTime(r.ts)}</span>
                      <Badge tone={kindTone[r.kind] ?? 'neutral'}>{r.kind}</Badge>
                      <span className="whitespace-pre-wrap break-all text-zinc-800 dark:text-zinc-200">{r.line}</span>
                    </div>
                  ))}
                </div>
              )}
            </Panel>
          )
        })
      )}
    </div>
  )
}
