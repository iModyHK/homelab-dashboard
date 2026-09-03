import { Link } from 'react-router-dom'
import { bytes, percent, rate } from '../lib/format'
import type { StackSummary } from '../lib/types'
import { Badge, Dot, stackTone } from './ui'

export function StackCard({ stack }: { stack: StackSummary }) {
  const tone = stackTone(stack.health)
  return (
    <Link
      to={`/stacks/${encodeURIComponent(stack.name)}`}
      className="group flex flex-col gap-2 rounded-xl border border-zinc-200 bg-white p-3.5 shadow-sm transition hover:border-zinc-400 dark:border-zinc-800 dark:bg-zinc-900 dark:hover:border-zinc-600"
    >
      <div className="flex items-center gap-2">
        <Dot tone={tone} pulse={stack.health === 'down'} title={stack.health} />
        <span className="truncate font-semibold tracking-tight">{stack.name}</span>
        <span className="ml-auto flex items-center gap-1">
          {stack.updates > 0 && <Badge tone="info">{stack.updates} update{stack.updates > 1 ? 's' : ''}</Badge>}
          <Badge tone="neutral" title={`grouped by ${stack.source}`}>
            {stack.source}
          </Badge>
        </span>
      </div>
      <div className="flex items-baseline gap-3 text-sm">
        <span className="tabular-nums">
          <span className="font-semibold">{stack.running}</span>
          <span className="text-zinc-500">/{stack.total} running</span>
        </span>
        {stack.unhealthy > 0 && <span className="text-xs text-red-600 dark:text-red-400">{stack.unhealthy} unhealthy</span>}
        {stack.exited > 0 && <span className="text-xs text-zinc-500">{stack.exited} exited</span>}
      </div>
      <div className="grid grid-cols-3 gap-2 text-xs tabular-nums text-zinc-600 dark:text-zinc-300">
        <div>
          <div className="text-[10px] uppercase tracking-wide text-zinc-500">CPU</div>
          {percent(stack.cpu, 1)}
        </div>
        <div>
          <div className="text-[10px] uppercase tracking-wide text-zinc-500">Memory</div>
          {bytes(stack.mem)}
        </div>
        <div>
          <div className="text-[10px] uppercase tracking-wide text-zinc-500">Net</div>
          <span title={`↓ ${rate(stack.netRx)} ↑ ${rate(stack.netTx)}`}>{rate(stack.netRx + stack.netTx)}</span>
        </div>
      </div>
    </Link>
  )
}
