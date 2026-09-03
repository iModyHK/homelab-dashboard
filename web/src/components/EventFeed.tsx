import { relative } from '../lib/format'
import type { EventRecord } from '../lib/types'
import { ContainerLink, Dot, Empty, type Tone } from './ui'

function eventTone(e: EventRecord): Tone {
  switch (e.type) {
    case 'start':
    case 'unpause':
      return 'good'
    case 'die':
      return e.detail === 'exit 0' ? 'neutral' : 'critical'
    case 'oom':
    case 'kill':
      return 'critical'
    case 'restart':
    case 'stop':
    case 'pause':
      return 'warning'
    case 'health_status':
      return e.detail === 'healthy' ? 'good' : 'critical'
    case 'destroy':
      return 'serious'
    default:
      return 'neutral'
  }
}

export function EventFeed({ events, now, limit = 30 }: { events: EventRecord[]; now: number; limit?: number }) {
  if (!events.length) return <Empty>No container events yet.</Empty>
  return (
    <ul className="divide-y divide-zinc-100 dark:divide-zinc-800/60">
      {events.slice(0, limit).map((e) => (
        <li key={e.id} className="flex items-center gap-2 py-1.5 text-sm">
          <Dot tone={eventTone(e)} />
          <ContainerLink id={e.containerId} name={e.containerName} className="truncate" />
          <span className="text-zinc-600 dark:text-zinc-300">
            {e.type === 'health_status' ? 'health' : e.type}
            {e.detail ? <span className="text-zinc-500"> · {e.detail}</span> : null}
          </span>
          <span className="ml-auto shrink-0 text-xs tabular-nums text-zinc-500">{relative(e.ts, now)}</span>
        </li>
      ))}
    </ul>
  )
}
