import { useMemo, useState } from 'react'
import { bytes, imageTag, percent, rate, uptimeSince } from '../lib/format'
import type { Container } from '../lib/types'
import { Sparkline } from './Sparkline'
import { Badge, ContainerLink, Dot, Empty, stateTone, UsageBar } from './ui'

type SortKey = 'name' | 'state' | 'cpu' | 'mem' | 'net' | 'restarts' | 'uptime'

export function ContainerTable({ containers, now, showStack = false }: { containers: Container[]; now: number; showStack?: boolean }) {
  const [sort, setSort] = useState<SortKey>('name')
  const [desc, setDesc] = useState(false)
  const [filter, setFilter] = useState('')
  const [onlyProblems, setOnlyProblems] = useState(false)

  const rows = useMemo(() => {
    const q = filter.trim().toLowerCase()
    let list = containers.filter((c) => !q || c.name.toLowerCase().includes(q) || c.image.toLowerCase().includes(q) || c.stack.toLowerCase().includes(q))
    if (onlyProblems) list = list.filter((c) => c.state !== 'running' || c.health === 'unhealthy' || c.updateAvailable)
    const val = (c: Container): number | string => {
      switch (sort) {
        case 'name':
          return c.name
        case 'state':
          return `${c.state === 'running' ? 0 : 1}${c.health === 'unhealthy' ? 0 : 1}${c.name}`
        case 'cpu':
          return c.live?.cpu ?? -1
        case 'mem':
          return c.live?.mem ?? -1
        case 'net':
          return (c.live?.netRx ?? 0) + (c.live?.netTx ?? 0)
        case 'restarts':
          return c.restartCount
        case 'uptime':
          return c.state === 'running' ? -c.startedAt : 1
      }
    }
    list = [...list].sort((a, b) => {
      const va = val(a)
      const vb = val(b)
      const r = typeof va === 'number' && typeof vb === 'number' ? va - vb : String(va).localeCompare(String(vb))
      return desc ? -r : r
    })
    return list
  }, [containers, sort, desc, filter, onlyProblems])

  const th = (key: SortKey, label: string, align = 'text-left') => (
    <th
      className={`cursor-pointer select-none whitespace-nowrap px-2 py-1.5 text-[11px] font-semibold uppercase tracking-wide text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200 ${align}`}
      onClick={() => {
        if (sort === key) setDesc((d) => !d)
        else {
          setSort(key)
          setDesc(key !== 'name' && key !== 'state')
        }
      }}
    >
      {label}
      {sort === key && <span className="ml-1">{desc ? '↓' : '↑'}</span>}
    </th>
  )

  return (
    <div>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <input
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Filter by name, image, stack"
          className="w-64 max-w-full rounded-md border border-zinc-300 bg-white px-2.5 py-1 text-sm outline-none focus:border-zinc-500 dark:border-zinc-700 dark:bg-zinc-950"
        />
        <label className="flex items-center gap-1.5 text-xs text-zinc-600 dark:text-zinc-300">
          <input type="checkbox" checked={onlyProblems} onChange={(e) => setOnlyProblems(e.target.checked)} />
          problems only
        </label>
        <span className="ml-auto text-xs text-zinc-500">
          {rows.length} of {containers.length}
        </span>
      </div>
      {rows.length === 0 ? (
        <Empty>No containers match.</Empty>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[760px] border-collapse text-sm">
            <thead className="border-b border-zinc-200 dark:border-zinc-800">
              <tr>
                {th('state', '')}
                {th('name', 'Name')}
                {showStack && <th className="px-2 py-1.5 text-left text-[11px] font-semibold uppercase tracking-wide text-zinc-500">Stack</th>}
                {th('uptime', 'Uptime')}
                {th('cpu', 'CPU', 'text-right')}
                {th('mem', 'Memory', 'text-right')}
                {th('net', 'Net ↓ / ↑', 'text-right')}
                {th('restarts', 'Restarts', 'text-right')}
                <th className="px-2 py-1.5 text-left text-[11px] font-semibold uppercase tracking-wide text-zinc-500">Image</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((c) => {
                const tone = stateTone(c.state, c.health)
                const memMax = c.memoryLimit > 0 ? c.memoryLimit : (c.live?.memLimit ?? 0)
                return (
                  <tr key={c.id} className="border-b border-zinc-100 hover:bg-zinc-50 dark:border-zinc-800/60 dark:hover:bg-zinc-800/40">
                    <td className="px-2 py-1.5">
                      <Dot tone={tone} pulse={c.state === 'restarting'} title={`${c.state}${c.health ? ` · ${c.health}` : ''}`} />
                    </td>
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-2">
                        <ContainerLink id={c.id} name={c.name} />
                        {c.health && c.health !== 'healthy' && <Badge tone={c.health === 'unhealthy' ? 'critical' : 'warning'}>{c.health}</Badge>}
                        {c.state !== 'running' && (
                          <Badge tone={tone}>
                            {c.state}
                            {c.state === 'exited' ? ` (${c.exitCode})` : ''}
                          </Badge>
                        )}
                      </div>
                    </td>
                    {showStack && <td className="px-2 py-1.5 text-xs text-zinc-500">{c.stack}</td>}
                    <td className="whitespace-nowrap px-2 py-1.5 text-xs tabular-nums text-zinc-600 dark:text-zinc-300">{c.state === 'running' ? uptimeSince(c.startedAt, now) : '—'}</td>
                    <td className="px-2 py-1.5 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <Sparkline values={c.sparkline ?? []} max={Math.max(100, ...(c.sparkline ?? []))} />
                        <span className="w-14 tabular-nums">{c.live ? percent(c.live.cpu, 1) : '—'}</span>
                      </div>
                    </td>
                    <td className="px-2 py-1.5 text-right">
                      <div className="flex items-center justify-end gap-2">
                        <div className="w-16">{c.live && memMax > 0 ? <UsageBar value={c.live.mem} max={memMax} label={`${bytes(c.live.mem)} of ${bytes(memMax)}`} /> : null}</div>
                        <span className="w-20 tabular-nums" title={c.memoryLimit > 0 ? `limit ${bytes(c.memoryLimit)}` : 'no limit'}>
                          {c.live ? bytes(c.live.mem) : '—'}
                        </span>
                      </div>
                    </td>
                    <td className="whitespace-nowrap px-2 py-1.5 text-right text-xs tabular-nums text-zinc-600 dark:text-zinc-300">{c.live ? `${rate(c.live.netRx)} / ${rate(c.live.netTx)}` : '—'}</td>
                    <td className="px-2 py-1.5 text-right tabular-nums">
                      <span className={c.restartCount > 0 ? 'text-amber-600 dark:text-amber-300' : ''}>{c.restartCount}</span>
                    </td>
                    <td className="max-w-[220px] px-2 py-1.5">
                      <div className="flex items-center gap-1.5">
                        <span className="truncate font-mono text-xs text-zinc-600 dark:text-zinc-300" title={c.image}>
                          {imageTag(c.image)}
                        </span>
                        {c.updateAvailable && <Badge tone="info" title="A newer image digest is available upstream">update</Badge>}
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
