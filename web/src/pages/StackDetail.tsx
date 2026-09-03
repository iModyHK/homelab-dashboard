import { Link, useParams } from 'react-router-dom'
import { ContainerTable } from '../components/ContainerTable'
import { Badge, Dot, Empty, Panel, stackTone } from '../components/ui'
import { bytes, percent, rate } from '../lib/format'
import { useDashboard } from '../store/DashboardProvider'

export function StackDetail() {
  const { name = '' } = useParams()
  const { overview, now } = useDashboard()
  if (!overview) return null
  const stack = overview.stacks.find((s) => s.name === name)
  const containers = overview.containers.filter((c) => c.stack === name)
  if (!stack) return <Empty>Stack "{name}" is not present. It may have been removed.</Empty>
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <Link to="/" className="text-sm text-zinc-500 hover:underline">
          Stacks
        </Link>
        <span className="text-zinc-400">/</span>
        <Dot tone={stackTone(stack.health)} />
        <h1 className="text-xl font-semibold tracking-tight">{stack.name}</h1>
        <Badge tone="neutral">{stack.source}</Badge>
        {stack.stackType && <Badge tone="neutral">{stack.stackType}</Badge>}
        {stack.portainerId > 0 && <span className="text-xs text-zinc-500">portainer stack #{stack.portainerId}</span>}
        <span className="ml-auto text-sm tabular-nums text-zinc-600 dark:text-zinc-300">
          {percent(stack.cpu, 1)} CPU · {bytes(stack.mem)} · ↓{rate(stack.netRx)} ↑{rate(stack.netTx)}
        </span>
      </div>
      <Panel>
        <ContainerTable containers={containers} now={now} />
      </Panel>
    </div>
  )
}
