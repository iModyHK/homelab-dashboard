import { bytes, duration, percent } from '../lib/format'
import type { Disk, MountUsage, RaidArray, TempPoint } from '../lib/types'
import { Sparkline } from './Sparkline'
import { Badge, Dot, Empty, Kv, type Tone, UsageBar } from './ui'

export function MountBars({ mounts }: { mounts: MountUsage[] }) {
  if (!mounts.length) return <Empty>No tracked mounts reported yet.</Empty>
  return (
    <div className="space-y-3">
      {mounts.map((m) => {
        const pct = m.Total ? (m.Used / m.Total) * 100 : 0
        return (
          <div key={m.Mount}>
            <div className="mb-1 flex items-baseline justify-between text-sm">
              <span className="font-mono">{m.Mount}</span>
              <span className="tabular-nums text-zinc-600 dark:text-zinc-300">
                {bytes(m.Used)} of {bytes(m.Total)} <span className="text-zinc-500">({percent(pct)})</span>
              </span>
            </div>
            <UsageBar value={m.Used} max={m.Total} />
          </div>
        )
      })}
    </div>
  )
}

export function RaidPanel({ arrays }: { arrays: RaidArray[] }) {
  if (!arrays.length) return <Empty>No mdadm arrays visible. Check that the host /proc is mounted.</Empty>
  return (
    <div className="space-y-2">
      {arrays.map((a) => {
        const tone: Tone = !a.active ? 'critical' : a.degraded ? 'critical' : a.syncAction ? 'warning' : 'good'
        return (
          <div key={a.name} className="flex flex-wrap items-center gap-2 rounded-lg border border-zinc-200 px-3 py-2 text-sm dark:border-zinc-800">
            <Dot tone={tone} pulse={a.degraded} />
            <span className="font-mono font-semibold">{a.name}</span>
            <Badge tone="neutral">{a.level}</Badge>
            <span className="text-zinc-600 dark:text-zinc-300">{a.state}</span>
            <span className="text-xs tabular-nums text-zinc-500">
              {a.slotsActive}/{a.slotsTotal} members · {bytes(a.blocks * 1024)}
            </span>
            {a.syncAction && (
              <span className="text-xs text-amber-600 dark:text-amber-300">
                {a.syncAction} {a.syncPercent.toFixed(1)}%{a.syncFinishIn ? `, ${a.syncFinishIn} left` : ''}
              </span>
            )}
            <span className="ml-auto flex flex-wrap gap-1">
              {(a.members ?? []).map((m) => (
                <span key={m.device} className={`rounded px-1.5 py-0.5 font-mono text-[11px] ${m.faulty ? 'bg-red-500/15 text-red-700 dark:text-red-300' : m.spare ? 'bg-zinc-500/15 text-zinc-500' : 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'}`} title={m.faulty ? 'faulty' : m.spare ? 'spare' : `slot ${m.slot}`}>
                  {m.device}
                </span>
              ))}
            </span>
          </div>
        )
      })}
    </div>
  )
}

export function DiskCard({ disk, temps, tempLimit }: { disk: Disk; temps: TempPoint[]; tempLimit: number }) {
  const healthy = disk.smartKnown ? disk.smartPassed && disk.reallocated === 0 && disk.pending === 0 && disk.uncorrectable === 0 : disk.reallocated === 0 && disk.pending === 0
  const tone: Tone = !healthy ? 'critical' : disk.standby ? 'neutral' : disk.temperature >= tempLimit ? 'warning' : 'good'
  const tempTone = disk.temperature >= tempLimit ? 'text-amber-600 dark:text-amber-300' : ''
  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900">
      <div className="mb-2 flex items-center gap-2">
        <Dot tone={tone} />
        <span className="font-mono font-semibold">{disk.device.replace('/dev/', '')}</span>
        <span className="truncate text-sm text-zinc-600 dark:text-zinc-300" title={disk.model}>
          {disk.model || 'unknown model'}
        </span>
        <span className="ml-auto">
          {disk.standby ? <Badge tone="neutral">standby</Badge> : disk.smartKnown ? <Badge tone={disk.smartPassed ? 'good' : 'critical'}>SMART {disk.smartPassed ? 'passed' : 'FAILED'}</Badge> : <Badge tone="neutral">no SMART</Badge>}
        </span>
      </div>
      <div className="mb-2 flex items-end gap-3">
        <div>
          <div className="text-[11px] uppercase tracking-wide text-zinc-500">Temp</div>
          <div className={`text-2xl font-semibold tabular-nums ${tempTone}`}>{disk.temperature ? `${disk.temperature}°C` : '—'}</div>
        </div>
        <div className="mb-1 flex-1">
          <Sparkline values={temps.map((t) => t.temp)} width={160} height={28} max={Math.max(tempLimit, ...temps.map((t) => t.temp))} />
        </div>
      </div>
      <div className="grid grid-cols-2 gap-x-4">
        <Kv k="Capacity" v={disk.capacityBytes ? bytes(disk.capacityBytes, 2) : '—'} />
        <Kv k="Power on" v={disk.powerOnHours ? duration(disk.powerOnHours * 3600) : '—'} />
        <Kv k="Reallocated" v={<span className={disk.reallocated ? 'text-red-600 dark:text-red-400' : ''}>{disk.reallocated}</span>} />
        <Kv k="Pending" v={<span className={disk.pending ? 'text-red-600 dark:text-red-400' : ''}>{disk.pending}</span>} />
        <Kv k="Uncorrectable" v={<span className={disk.uncorrectable ? 'text-red-600 dark:text-red-400' : ''}>{disk.uncorrectable}</span>} />
        <Kv k="CRC errors" v={<span className={disk.crcErrors ? 'text-amber-600 dark:text-amber-300' : ''}>{disk.crcErrors}</span>} />
        <Kv k="Serial" v={disk.serial || '—'} />
        <Kv k="Firmware" v={disk.firmware || '—'} />
        {disk.transport === 'NVMe' && <Kv k="Life used" v={`${disk.percentUsed}%`} />}
        {disk.rotationRpm > 0 && <Kv k="Rotation" v={`${disk.rotationRpm} rpm`} />}
      </div>
    </div>
  )
}
