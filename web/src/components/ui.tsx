import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'

export function Panel({ title, action, children, className = '' }: { title?: ReactNode; action?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`rounded-xl border border-zinc-200 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-900 ${className}`}>
      {(title || action) && (
        <header className="flex items-center justify-between gap-3 border-b border-zinc-200 px-4 py-2.5 dark:border-zinc-800">
          <h2 className="text-sm font-semibold tracking-tight text-zinc-700 dark:text-zinc-200">{title}</h2>
          {action}
        </header>
      )}
      <div className="p-4">{children}</div>
    </section>
  )
}

export type Tone = 'good' | 'warning' | 'serious' | 'critical' | 'neutral' | 'info'

const toneDot: Record<Tone, string> = {
  good: 'bg-emerald-500',
  warning: 'bg-amber-400',
  serious: 'bg-orange-500',
  critical: 'bg-red-500',
  neutral: 'bg-zinc-400 dark:bg-zinc-600',
  info: 'bg-sky-500',
}

const toneBadge: Record<Tone, string> = {
  good: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
  warning: 'bg-amber-400/20 text-amber-800 dark:text-amber-200',
  serious: 'bg-orange-500/15 text-orange-700 dark:text-orange-300',
  critical: 'bg-red-500/15 text-red-700 dark:text-red-300',
  neutral: 'bg-zinc-500/15 text-zinc-600 dark:text-zinc-300',
  info: 'bg-sky-500/15 text-sky-700 dark:text-sky-300',
}

export function Dot({ tone, pulse = false, title }: { tone: Tone; pulse?: boolean; title?: string }) {
  return (
    <span className="relative inline-flex h-2.5 w-2.5" title={title}>
      {pulse && <span className={`absolute inline-flex h-full w-full animate-ping rounded-full opacity-60 ${toneDot[tone]}`} />}
      <span className={`relative inline-flex h-2.5 w-2.5 rounded-full ${toneDot[tone]}`} />
    </span>
  )
}

export function Badge({ tone, children, title }: { tone: Tone; children: ReactNode; title?: string }) {
  return (
    <span title={title} className={`inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-medium leading-4 ${toneBadge[tone]}`}>
      {children}
    </span>
  )
}

export function stateTone(state: string, health: string): Tone {
  if (health === 'unhealthy') return 'critical'
  if (health === 'starting') return 'warning'
  switch (state) {
    case 'running':
      return 'good'
    case 'restarting':
      return 'serious'
    case 'paused':
      return 'info'
    case 'exited':
    case 'dead':
      return 'critical'
    default:
      return 'neutral'
  }
}

export function stackTone(health: string): Tone {
  switch (health) {
    case 'healthy':
      return 'good'
    case 'partial':
      return 'warning'
    case 'unhealthy':
      return 'critical'
    case 'down':
      return 'critical'
    default:
      return 'neutral'
  }
}

export function UsageBar({ value, max, tone, label }: { value: number; max: number; tone?: Tone; label?: string }) {
  const pct = max > 0 ? Math.min(100, (value / max) * 100) : 0
  const auto: Tone = pct >= 90 ? 'critical' : pct >= 75 ? 'warning' : 'info'
  const t = tone ?? auto
  return (
    <div className="flex items-center gap-2" title={label}>
      <div className="h-1.5 w-full min-w-10 overflow-hidden rounded-full bg-zinc-200 dark:bg-zinc-800">
        <div className={`h-full rounded-full ${toneDot[t]}`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

export function Stat({ label, value, sub, tone }: { label: string; value: ReactNode; sub?: ReactNode; tone?: Tone }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] font-medium uppercase tracking-wide text-zinc-500">{label}</div>
      <div className={`truncate text-lg font-semibold tabular-nums leading-tight ${tone === 'critical' ? 'text-red-600 dark:text-red-400' : tone === 'warning' ? 'text-amber-600 dark:text-amber-300' : ''}`}>
        {value}
      </div>
      {sub && <div className="truncate text-xs text-zinc-500">{sub}</div>}
    </div>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="rounded-lg border border-dashed border-zinc-300 p-6 text-center text-sm text-zinc-500 dark:border-zinc-700">{children}</div>
}

export function Spinner() {
  return <div className="h-5 w-5 animate-spin rounded-full border-2 border-zinc-300 border-t-zinc-700 dark:border-zinc-700 dark:border-t-zinc-200" />
}

export function ContainerLink({ id, name, className = '' }: { id: string; name: string; className?: string }) {
  return (
    <Link to={`/containers/${id.slice(0, 12)}`} className={`font-medium text-zinc-900 hover:underline dark:text-zinc-100 ${className}`}>
      {name}
    </Link>
  )
}

export function Button({ children, onClick, active = false, small = false, disabled = false, type = 'button', title }: {
  children: ReactNode
  onClick?: () => void
  active?: boolean
  small?: boolean
  disabled?: boolean
  type?: 'button' | 'submit'
  title?: string
}) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={`rounded-md border font-medium transition ${small ? 'px-2 py-0.5 text-xs' : 'px-3 py-1.5 text-sm'} ${
        active
          ? 'border-zinc-900 bg-zinc-900 text-white dark:border-zinc-100 dark:bg-zinc-100 dark:text-zinc-900'
          : 'border-zinc-300 bg-white text-zinc-700 hover:bg-zinc-100 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-200 dark:hover:bg-zinc-800'
      } disabled:cursor-not-allowed disabled:opacity-50`}
    >
      {children}
    </button>
  )
}

export function Segmented<T extends string>({ value, options, onChange }: { value: T; options: readonly T[]; onChange: (v: T) => void }) {
  return (
    <div className="inline-flex overflow-hidden rounded-md border border-zinc-300 dark:border-zinc-700">
      {options.map((o) => (
        <button
          key={o}
          type="button"
          onClick={() => onChange(o)}
          className={`px-2.5 py-1 text-xs font-medium ${o === value ? 'bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900' : 'bg-white text-zinc-600 hover:bg-zinc-100 dark:bg-zinc-900 dark:text-zinc-300 dark:hover:bg-zinc-800'}`}
        >
          {o}
        </button>
      ))}
    </div>
  )
}

export function Kv({ k, v }: { k: string; v: ReactNode }) {
  return (
    <div className="flex justify-between gap-4 py-1 text-sm">
      <span className="shrink-0 text-zinc-500">{k}</span>
      <span className="truncate text-right font-mono text-xs text-zinc-800 dark:text-zinc-200" title={typeof v === 'string' ? v : undefined}>
        {v}
      </span>
    </div>
  )
}
