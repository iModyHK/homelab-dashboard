import { Link } from 'react-router-dom'
import { EventFeed } from '../components/EventFeed'
import { HostStrip } from '../components/HostStrip'
import { StackCard } from '../components/StackCard'
import { MountBars, RaidPanel } from '../components/Storage'
import { Badge, Dot, Empty, Panel, Spinner, type Tone } from '../components/ui'
import { useDashboard } from '../store/DashboardProvider'

export function Overview() {
  const { overview, loading, error, now } = useDashboard()
  if (loading && !overview) {
    return (
      <div className="flex justify-center p-12">
        <Spinner />
      </div>
    )
  }
  if (!overview) return <Empty>{error || 'Nothing loaded yet.'}</Empty>

  const hostStale = !overview.sources.host?.ok
  const diskTone = (): Tone => {
    if (overview.arrays.some((a) => a.degraded || !a.active)) return 'critical'
    if (overview.disks.some((d) => (d.smartKnown && !d.smartPassed) || d.reallocated > 0 || d.pending > 0)) return 'critical'
    if (overview.arrays.some((a) => a.syncAction)) return 'warning'
    return overview.disks.length || overview.arrays.length ? 'good' : 'neutral'
  }

  return (
    <div className="space-y-4">
      <HostStrip host={overview.host} stale={hostStale} />
      {hostStale && overview.sources.host && (
        <div className="rounded-lg border border-amber-400/40 bg-amber-400/10 px-3 py-2 text-xs text-amber-800 dark:text-amber-200">
          Host metrics unavailable: {overview.sources.host.lastError.split('\n')[0]}
        </div>
      )}

      <section>
        <div className="mb-2 flex items-center gap-2">
          <h2 className="text-sm font-semibold tracking-tight text-zinc-700 dark:text-zinc-200">Stacks</h2>
          <span className="text-xs text-zinc-500">
            {overview.stacks.length} stacks · {overview.containers.filter((c) => c.state === 'running').length}/{overview.containers.length} containers running
          </span>
          {overview.portainer.endpoints?.map((e) => (
            <Badge key={e.id} tone={e.online ? 'good' : 'critical'} title={`Portainer endpoint ${e.id}`}>
              {e.name}
            </Badge>
          ))}
        </div>
        {overview.stacks.length === 0 ? (
          <Empty>No containers discovered yet.</Empty>
        ) : (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {overview.stacks.map((s) => (
              <StackCard key={s.name} stack={s} />
            ))}
          </div>
        )}
      </section>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Panel
          title={
            <span className="flex items-center gap-2">
              <Dot tone={diskTone()} /> Storage
            </span>
          }
          action={
            <Link to="/disks" className="text-xs text-sky-600 hover:underline dark:text-sky-400">
              details →
            </Link>
          }
        >
          <div className="space-y-4">
            <MountBars mounts={overview.host.mounts ?? []} />
            <RaidPanel arrays={overview.arrays} />
            {overview.disks.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {overview.disks.map((d) => {
                  const bad = (d.smartKnown && !d.smartPassed) || d.reallocated > 0 || d.pending > 0
                  return (
                    <span key={d.device} className={`rounded-md px-2 py-1 font-mono text-xs ${bad ? 'bg-red-500/15 text-red-700 dark:text-red-300' : 'bg-zinc-100 text-zinc-700 dark:bg-zinc-800 dark:text-zinc-200'}`} title={d.model}>
                      {d.device.replace('/dev/', '')} {d.standby ? 'zz' : d.temperature ? `${d.temperature}°` : ''}
                    </span>
                  )
                })}
              </div>
            )}
          </div>
        </Panel>
        <Panel title="Recent events" className="lg:col-span-2">
          <EventFeed events={overview.events} now={now} />
        </Panel>
      </div>
    </div>
  )
}
