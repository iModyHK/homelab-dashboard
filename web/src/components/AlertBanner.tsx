import { Link } from 'react-router-dom'
import { api } from '../lib/api'
import { relative } from '../lib/format'
import { useDashboard } from '../store/DashboardProvider'
import { Badge, Button } from './ui'

export const RULE_TITLES: Record<string, string> = {
  container_down: 'Container down',
  restart_loop: 'Restart loop',
  health_failing: 'Health check failing',
  cpu_high: 'CPU high',
  memory_high: 'Memory high',
  disk_high: 'Disk almost full',
  smart_failing: 'SMART failure',
  disk_temp_high: 'Drive temperature high',
  raid_degraded: 'RAID degraded',
  host_unreachable: 'Host unreachable',
  db_size: 'Database large',
  host_cpu_high: 'Host CPU high',
  host_memory_high: 'Host memory high',
}

export function AlertBanner() {
  const { overview, now } = useDashboard()
  const active = overview?.alerts.filter((a) => !a.ackedAt) ?? []
  if (!active.length) return null
  const ack = (id: number) => api.post(`/api/alerts/${id}/ack`).catch(() => undefined)
  return (
    <div className="border-b border-red-500/30 bg-red-500/10">
      <div className="mx-auto flex max-w-[1600px] flex-col gap-1 px-4 py-2">
        {active.slice(0, 3).map((a) => (
          <div key={a.id} className="flex flex-wrap items-center gap-2 text-sm">
            <Badge tone={a.severity === 'critical' ? 'critical' : 'warning'}>{a.severity}</Badge>
            <span className="font-medium">{RULE_TITLES[a.rule] ?? a.rule}</span>
            <span className="text-zinc-700 dark:text-zinc-300">
              {a.targetName} · {a.message}
            </span>
            <span className="text-xs text-zinc-500">{relative(a.firedAt, now)}</span>
            <span className="ml-auto">
              <Button small onClick={() => void ack(a.id)}>
                Acknowledge
              </Button>
            </span>
          </div>
        ))}
        {active.length > 3 && (
          <Link to="/alerts" className="text-xs text-red-700 hover:underline dark:text-red-300">
            {active.length - 3} more active alerts
          </Link>
        )}
      </div>
    </div>
  )
}
