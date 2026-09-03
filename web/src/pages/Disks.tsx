import { useEffect, useState } from 'react'
import { SeriesChart } from '../components/SeriesChart'
import { DiskCard, MountBars, RaidPanel } from '../components/Storage'
import { Empty, Panel, Segmented, Spinner } from '../components/ui'
import { api } from '../lib/api'
import { bytes, percent, rangeSeconds, rate } from '../lib/format'
import type { Theme } from '../lib/theme'
import { RANGES, type Disk, type HostPoint, type MountPoint, type MountUsage, type RaidArray, type Range, type SourceStatus, type TempPoint } from '../lib/types'
import { useDashboard } from '../store/DashboardProvider'

interface DisksResponse {
  disks: Disk[]
  arrays: RaidArray[]
  mounts: MountUsage[]
  temps: Record<string, TempPoint[]>
  smart?: SourceStatus
}

export function Disks({ theme }: { theme: Theme }) {
  const { overview } = useDashboard()
  const [data, setData] = useState<DisksResponse | null>(null)
  const [range, setRange] = useState<Range>('24h')
  const [host, setHost] = useState<{ points: HostPoint[]; mounts: MountPoint[] } | null>(null)

  useEffect(() => {
    let cancelled = false
    const load = () => {
      api.get<DisksResponse>(`/api/disks?range=${range}`).then((r) => !cancelled && setData(r)).catch(() => undefined)
      api.get<{ points: HostPoint[]; mounts: MountPoint[] }>(`/api/host/series?range=${range}`).then((r) => !cancelled && setHost(r)).catch(() => undefined)
    }
    load()
    const t = setInterval(load, 60000)
    return () => {
      cancelled = true
      clearInterval(t)
    }
  }, [range])

  if (!data) {
    return (
      <div className="flex justify-center p-12">
        <Spinner />
      </div>
    )
  }
  const disks = overview?.disks.length ? overview.disks : data.disks
  const arrays = overview?.arrays.length ? overview.arrays : data.arrays
  const mounts = overview?.host.mounts ?? data.mounts
  const span = rangeSeconds(range)
  const tempLimit = 55
  const mountNames = [...new Set((host?.mounts ?? []).map((m) => m.mount))]
  const mountSeries = mountNames.map((m, i) => ({ key: m, label: m, color: i }))
  const mountData = Object.values(
    (host?.mounts ?? []).reduce<Record<number, Record<string, number>>>((acc, p) => {
      acc[p.ts] = acc[p.ts] ?? { ts: p.ts }
      acc[p.ts][p.mount] = p.total ? (p.used / p.total) * 100 : 0
      return acc
    }, {}),
  )

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-xl font-semibold tracking-tight">Storage</h1>
        {data.smart && !data.smart.ok && <span className="text-xs text-amber-700 dark:text-amber-300">SMART sidecar: {data.smart.lastError}</span>}
        <span className="ml-auto">
          <Segmented value={range} options={RANGES} onChange={setRange} />
        </span>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Panel title="Mounts">
          <MountBars mounts={mounts ?? []} />
        </Panel>
        <Panel title="RAID arrays">
          <RaidPanel arrays={arrays} />
        </Panel>
      </div>

      {disks.length === 0 ? (
        <Empty>No SMART data. Start the stack with the smart profile to enable per-drive monitoring.</Empty>
      ) : (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
          {disks.map((d) => (
            <DiskCard key={d.device} disk={d} temps={data.temps[d.device] ?? []} tempLimit={tempLimit} />
          ))}
        </div>
      )}

      <Panel title="Host history">
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          <div>
            <div className="mb-1 text-xs font-medium text-zinc-500">Host CPU %</div>
            <SeriesChart data={host?.points ?? []} series={[{ key: 'cpu', label: 'avg' }, { key: 'cpuMax', label: 'max', color: 1 }]} rangeSeconds={span} theme={theme} format={(v) => percent(v)} yMax={100} />
          </div>
          <div>
            <div className="mb-1 text-xs font-medium text-zinc-500">Host memory</div>
            <SeriesChart data={host?.points ?? []} series={[{ key: 'memUsed', label: 'used', color: 2 }, { key: 'swapUsed', label: 'swap', color: 3 }]} rangeSeconds={span} theme={theme} format={(v) => bytes(v, 0)} />
          </div>
          <div>
            <div className="mb-1 text-xs font-medium text-zinc-500">Host network</div>
            <SeriesChart data={host?.points ?? []} series={[{ key: 'netRx', label: 'rx' }, { key: 'netTx', label: 'tx', color: 3 }]} rangeSeconds={span} theme={theme} format={(v) => rate(v)} />
          </div>
          <div>
            <div className="mb-1 text-xs font-medium text-zinc-500">Mount usage %</div>
            <SeriesChart data={mountData} series={mountSeries} rangeSeconds={span} theme={theme} format={(v) => percent(v)} yMax={100} />
          </div>
        </div>
      </Panel>
    </div>
  )
}
