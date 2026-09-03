import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { EventFeed } from '../components/EventFeed'
import { LogViewer } from '../components/LogViewer'
import { SeriesChart } from '../components/SeriesChart'
import { Badge, Dot, Empty, Kv, Panel, Segmented, Spinner, stateTone } from '../components/ui'
import { api } from '../lib/api'
import { bytes, dateTime, percent, rate, rangeSeconds, uptimeSince } from '../lib/format'
import type { Theme } from '../lib/theme'
import { RANGES, type ContainerDetail as Detail, type EventRecord, type Range, type SeriesPoint } from '../lib/types'
import { useDashboard } from '../store/DashboardProvider'

export function ContainerDetail({ theme }: { theme: Theme }) {
  const { id = '' } = useParams()
  const { overview, now } = useDashboard()
  const [detail, setDetail] = useState<Detail | null>(null)
  const [events, setEvents] = useState<EventRecord[]>([])
  const [gone, setGone] = useState(false)
  const [error, setError] = useState('')
  const [range, setRange] = useState<Range>('1h')
  const [points, setPoints] = useState<SeriesPoint[]>([])
  const [tab, setTab] = useState<'logs' | 'config'>('logs')

  const live = useMemo(() => overview?.containers.find((c) => c.id.startsWith(id)), [overview, id])

  useEffect(() => {
    let cancelled = false
    api
      .get<{ container: Detail; events?: EventRecord[]; gone?: boolean }>(`/api/containers/${id}`)
      .then((r) => {
        if (cancelled) return
        setDetail(r.container)
        setEvents(r.events ?? [])
        setGone(Boolean(r.gone))
      })
      .catch((e) => !cancelled && setError(e instanceof Error ? e.message : 'not found'))
    return () => {
      cancelled = true
    }
  }, [id, live?.state, live?.health, live?.restartCount])

  useEffect(() => {
    let cancelled = false
    const load = () =>
      api
        .get<{ points: SeriesPoint[] }>(`/api/containers/${id}/series?range=${range}`)
        .then((r) => !cancelled && setPoints(r.points))
        .catch(() => undefined)
    void load()
    const t = setInterval(() => void load(), range === '1h' ? 15000 : 60000)
    return () => {
      cancelled = true
      clearInterval(t)
    }
  }, [id, range])

  useEffect(() => {
    if (!overview) return
    setEvents((prev) => {
      const mine = overview.events.filter((e) => e.containerId.startsWith(id))
      const ids = new Set(prev.map((e) => e.id))
      const fresh = mine.filter((e) => !ids.has(e.id))
      return fresh.length ? [...fresh, ...prev].slice(0, 50) : prev
    })
  }, [overview, id])

  if (error) return <Empty>{error}</Empty>
  if (!detail) {
    return (
      <div className="flex justify-center p-12">
        <Spinner />
      </div>
    )
  }

  const c = live ? { ...detail, ...live } : detail
  const tone = stateTone(c.state, c.health)
  const span = rangeSeconds(range)
  const memMax = c.memoryLimit > 0 ? c.memoryLimit : c.live?.memLimit
  const data = points.map((p) => ({ ...p, netRx: p.netRx, netTx: p.netTx }))
  const restarts = events.filter((e) => e.type === 'die' || e.type === 'restart' || e.type === 'oom')

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Link to="/" className="text-sm text-zinc-500 hover:underline">
          Stacks
        </Link>
        <span className="text-zinc-400">/</span>
        <Link to={`/stacks/${encodeURIComponent(c.stack)}`} className="text-sm text-zinc-500 hover:underline">
          {c.stack}
        </Link>
        <span className="text-zinc-400">/</span>
        <Dot tone={tone} pulse={c.state === 'restarting'} />
        <h1 className="max-w-full break-all text-xl font-semibold tracking-tight">{c.name}</h1>
        <Badge tone={tone}>{c.state}</Badge>
        {c.health && <Badge tone={c.health === 'healthy' ? 'good' : c.health === 'unhealthy' ? 'critical' : 'warning'}>{c.health}</Badge>}
        {c.updateAvailable && <Badge tone="info">update available</Badge>}
        {gone && <Badge tone="serious">removed</Badge>}
        <span className="font-mono text-xs text-zinc-500 sm:ml-auto">{c.id.slice(0, 12)}</span>
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 lg:grid-cols-6">
        <Panel>
          <div className="text-[11px] uppercase tracking-wide text-zinc-500">CPU</div>
          <div className="text-xl font-semibold tabular-nums">{c.live ? percent(c.live.cpu, 1) : '—'}</div>
          <div className="text-xs text-zinc-500">{c.cpuLimit > 0 ? `limit ${c.cpuLimit} cores` : 'no limit'}</div>
        </Panel>
        <Panel>
          <div className="text-[11px] uppercase tracking-wide text-zinc-500">Memory</div>
          <div className="text-xl font-semibold tabular-nums">{c.live ? bytes(c.live.mem) : '—'}</div>
          <div className="text-xs text-zinc-500">{c.memoryLimit > 0 ? `limit ${bytes(c.memoryLimit)}` : 'no limit'}</div>
        </Panel>
        <Panel>
          <div className="text-[11px] uppercase tracking-wide text-zinc-500">Network</div>
          <div className="text-base font-semibold tabular-nums">↓ {c.live ? rate(c.live.netRx) : '—'}</div>
          <div className="text-xs tabular-nums text-zinc-500">↑ {c.live ? rate(c.live.netTx) : '—'}</div>
        </Panel>
        <Panel>
          <div className="text-[11px] uppercase tracking-wide text-zinc-500">Block I/O</div>
          <div className="text-base font-semibold tabular-nums">R {c.live ? rate(c.live.blkRead) : '—'}</div>
          <div className="text-xs tabular-nums text-zinc-500">W {c.live ? rate(c.live.blkWrite) : '—'}</div>
        </Panel>
        <Panel>
          <div className="text-[11px] uppercase tracking-wide text-zinc-500">Uptime</div>
          <div className="text-xl font-semibold tabular-nums">{c.state === 'running' ? uptimeSince(c.startedAt, now) : '—'}</div>
          <div className="text-xs text-zinc-500">{c.state === 'running' ? `since ${dateTime(c.startedAt)}` : c.finishedAt ? `stopped ${dateTime(c.finishedAt)}` : ''}</div>
        </Panel>
        <Panel>
          <div className="text-[11px] uppercase tracking-wide text-zinc-500">Restarts</div>
          <div className={`text-xl font-semibold tabular-nums ${c.restartCount > 0 ? 'text-amber-600 dark:text-amber-300' : ''}`}>{c.restartCount}</div>
          <div className="text-xs text-zinc-500">
            {c.restartPolicy || 'no'} policy · exit {c.exitCode}
            {c.oomKilled ? ' · OOM' : ''}
          </div>
        </Panel>
      </div>

      <Panel title="Usage" action={<Segmented value={range} options={RANGES} onChange={setRange} />}>
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <div>
            <div className="mb-1 text-xs font-medium text-zinc-500">CPU %</div>
            <SeriesChart data={data} series={[{ key: 'cpu', label: 'avg' }, { key: 'cpuMax', label: 'max', color: 1 }]} rangeSeconds={span} theme={theme} format={(v) => percent(v, 1)} />
          </div>
          <div>
            <div className="mb-1 text-xs font-medium text-zinc-500">Memory{memMax ? ` (limit ${bytes(memMax)})` : ''}</div>
            <SeriesChart data={data} series={[{ key: 'mem', label: 'used', color: 2 }]} rangeSeconds={span} theme={theme} format={(v) => bytes(v, 0)} yMax={c.memoryLimit > 0 ? c.memoryLimit : undefined} />
          </div>
          <div>
            <div className="mb-1 text-xs font-medium text-zinc-500">Network</div>
            <SeriesChart data={data} series={[{ key: 'netRx', label: 'rx' }, { key: 'netTx', label: 'tx', color: 3 }]} rangeSeconds={span} theme={theme} format={(v) => rate(v)} />
          </div>
          <div>
            <div className="mb-1 text-xs font-medium text-zinc-500">Block I/O</div>
            <SeriesChart data={data} series={[{ key: 'blkRead', label: 'read', color: 4 }, { key: 'blkWrite', label: 'write', color: 5 }]} rangeSeconds={span} theme={theme} format={(v) => rate(v)} />
          </div>
        </div>
      </Panel>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Panel title="Restart history" className="lg:col-span-1">
          {restarts.length === 0 ? <Empty>No restarts or non-zero exits recorded.</Empty> : <EventFeed events={restarts} now={now} limit={20} />}
          {c.error && <div className="mt-2 rounded-md bg-red-500/10 p-2 font-mono text-xs text-red-700 dark:text-red-300">{c.error}</div>}
        </Panel>
        <Panel title="Health checks" className="lg:col-span-2">
          {!c.healthLog?.length ? (
            <Empty>{c.health ? 'No health check output captured yet.' : 'This container has no health check.'}</Empty>
          ) : (
            <div className="space-y-2">
              {c.failingStreak > 0 && <div className="text-xs text-red-600 dark:text-red-400">Failing streak: {c.failingStreak}</div>}
              {[...c.healthLog].reverse().map((h, i) => (
                <div key={i} className="rounded-md border border-zinc-200 p-2 text-xs dark:border-zinc-800">
                  <div className="mb-1 flex items-center gap-2">
                    <Dot tone={h.exitCode === 0 ? 'good' : 'critical'} />
                    <span className="tabular-nums text-zinc-500">{dateTime(h.start)}</span>
                    <span>exit {h.exitCode}</span>
                  </div>
                  <pre className="whitespace-pre-wrap break-all font-mono text-[11px] text-zinc-700 dark:text-zinc-300">{h.output || '(no output)'}</pre>
                </div>
              ))}
            </div>
          )}
        </Panel>
      </div>

      <Panel
        title={
          <span className="flex gap-3">
            <button type="button" onClick={() => setTab('logs')} className={tab === 'logs' ? 'text-zinc-900 dark:text-zinc-100' : 'text-zinc-400'}>
              Logs
            </button>
            <button type="button" onClick={() => setTab('config')} className={tab === 'config' ? 'text-zinc-900 dark:text-zinc-100' : 'text-zinc-400'}>
              Configuration
            </button>
          </span>
        }
      >
        {tab === 'logs' ? (
          <LogViewer containerId={c.id} running={c.state === 'running'} />
        ) : (
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <div>
              <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-zinc-500">Image</h3>
              <Kv k="Reference" v={c.image} />
              <Kv k="Image ID" v={c.imageId.replace('sha256:', '').slice(0, 12)} />
              <Kv k="Created" v={dateTime(c.created)} />
              <Kv k="Service" v={c.service || '—'} />
              <h3 className="mb-1 mt-4 text-xs font-semibold uppercase tracking-wide text-zinc-500">Ports</h3>
              {c.ports?.length ? c.ports.map((p, i) => <Kv key={i} k={`${p.hostIp || '0.0.0.0'}:${p.hostPort}`} v={`→ ${p.containerPort}/${p.protocol}`} />) : <div className="text-sm text-zinc-500">none published</div>}
              <h3 className="mb-1 mt-4 text-xs font-semibold uppercase tracking-wide text-zinc-500">Networks</h3>
              {c.networks && Object.keys(c.networks).length ? Object.entries(c.networks).map(([n, ip]) => <Kv key={n} k={n} v={ip || '—'} />) : <div className="text-sm text-zinc-500">none</div>}
              <h3 className="mb-1 mt-4 text-xs font-semibold uppercase tracking-wide text-zinc-500">Mounts</h3>
              {c.mounts?.length ? (
                c.mounts.map((m, i) => (
                  <div key={i} className="py-1 font-mono text-xs">
                    <span className="text-zinc-500">{m.type}</span> {m.source} <span className="text-zinc-400">→</span> {m.destination}
                    {m.readOnly && <span className="ml-1 text-zinc-500">ro</span>}
                  </div>
                ))
              ) : (
                <div className="text-sm text-zinc-500">none</div>
              )}
            </div>
            <div>
              <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-zinc-500">Environment (secrets masked)</h3>
              <div className="max-h-96 overflow-auto rounded-md bg-zinc-50 p-2 font-mono text-[11px] dark:bg-zinc-950">
                {c.env?.length ? c.env.map((e, i) => <div key={i} className="break-all">{e}</div>) : <span className="text-zinc-500">none</span>}
              </div>
              <h3 className="mb-1 mt-4 text-xs font-semibold uppercase tracking-wide text-zinc-500">Labels</h3>
              <div className="max-h-64 overflow-auto rounded-md bg-zinc-50 p-2 font-mono text-[11px] dark:bg-zinc-950">
                {c.labels && Object.keys(c.labels).length ? (
                  Object.entries(c.labels)
                    .sort(([a], [b]) => a.localeCompare(b))
                    .map(([k, v]) => (
                      <div key={k} className="break-all">
                        <span className="text-zinc-500">{k}</span>={v}
                      </div>
                    ))
                ) : (
                  <span className="text-zinc-500">none</span>
                )}
              </div>
            </div>
          </div>
        )}
      </Panel>
    </div>
  )
}
