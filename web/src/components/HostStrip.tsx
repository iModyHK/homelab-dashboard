import { Link } from 'react-router-dom'
import { bytes, duration, percent, rate } from '../lib/format'
import type { HostState } from '../lib/types'
import { Stat, UsageBar } from './ui'

export function HostStrip({ host, stale }: { host: HostState; stale: boolean }) {
  const memPct = host.memTotal ? (host.memUsed / host.memTotal) * 100 : 0
  const swapPct = host.swapTotal ? (host.swapUsed / host.swapTotal) * 100 : 0
  const loadTone = host.cpus && host.load1 > host.cpus ? 'critical' : host.cpus && host.load1 > host.cpus * 0.7 ? 'warning' : undefined
  return (
    <section className={`grid grid-cols-2 gap-4 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm sm:grid-cols-3 lg:grid-cols-6 dark:border-zinc-800 dark:bg-zinc-900 ${stale ? 'opacity-60' : ''}`}>
      <div className="col-span-2 sm:col-span-3 lg:col-span-1">
        <div className="text-[11px] font-medium uppercase tracking-wide text-zinc-500">Host</div>
        <div className="truncate text-lg font-semibold leading-tight">{host.hostname || '—'}</div>
        <div className="truncate text-xs text-zinc-500" title={`${host.os} · ${host.kernel}`}>
          {host.cpus} cores · {host.os || 'unknown os'}
        </div>
      </div>
      <div>
        <Stat label="CPU" value={percent(host.cpu)} sub={host.cpuTemp ? `${host.cpuTemp.toFixed(0)}°C` : undefined} tone={host.cpu >= 90 ? 'critical' : host.cpu >= 75 ? 'warning' : undefined} />
        <UsageBar value={host.cpu} max={100} />
      </div>
      <div>
        <Stat label="Memory" value={host.memTotal ? percent(memPct) : '—'} sub={host.memTotal ? `${bytes(host.memUsed)} of ${bytes(host.memTotal)}` : 'not readable'} tone={memPct >= 90 ? 'critical' : memPct >= 75 ? 'warning' : undefined} />
        <UsageBar value={host.memUsed} max={host.memTotal} />
      </div>
      <div>
        <Stat label="Swap" value={host.swapTotal ? percent(swapPct) : '—'} sub={host.swapTotal ? `${bytes(host.swapUsed)} of ${bytes(host.swapTotal)}` : 'none'} tone={swapPct >= 90 ? 'warning' : undefined} />
        <UsageBar value={host.swapUsed} max={host.swapTotal} tone={swapPct >= 90 ? 'warning' : 'neutral'} />
      </div>
      <div>
        <Stat label="Load" value={`${host.load1.toFixed(2)}`} sub={`${host.load5.toFixed(2)} · ${host.load15.toFixed(2)}`} tone={loadTone} />
      </div>
      <div>
        <Stat label="Network" value={<span className="text-base">↓ {rate(host.netRx)}</span>} sub={`↑ ${rate(host.netTx)} · up ${duration(host.uptime)}`} />
        <Link to="/disks" className="text-xs text-sky-600 hover:underline dark:text-sky-400">
          storage →
        </Link>
      </div>
    </section>
  )
}
