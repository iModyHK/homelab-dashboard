let timezone = 'UTC'

export function setTimezone(tz: string) {
  if (tz) timezone = tz
}

const UNITS = ['B', 'KiB', 'MiB', 'GiB', 'TiB', 'PiB']

export function bytes(n: number, digits = 1): string {
  if (!Number.isFinite(n) || n < 0) return '—'
  let v = n
  let i = 0
  while (v >= 1024 && i < UNITS.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : digits)} ${UNITS[i]}`
}

export function rate(n: number): string {
  return `${bytes(n)}/s`
}

export function percent(n: number, digits = 0): string {
  if (!Number.isFinite(n)) return '—'
  return `${n.toFixed(digits)}%`
}

export function duration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '—'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h`
  if (h > 0) return `${h}h ${m}m`
  if (m > 0) return `${m}m`
  return `${Math.floor(seconds)}s`
}

export function uptimeSince(startedAt: number, nowSeconds = Date.now() / 1000): string {
  if (!startedAt) return '—'
  return duration(nowSeconds - startedAt)
}

export function relative(ts: number, nowSeconds = Date.now() / 1000): string {
  if (!ts) return '—'
  const diff = nowSeconds - ts
  if (diff < 5) return 'just now'
  if (diff < 60) return `${Math.floor(diff)}s ago`
  if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
  if (diff < 86400) return `${Math.floor(diff / 3600)}h ago`
  return `${Math.floor(diff / 86400)}d ago`
}

export function dateTime(ts: number, opts: Intl.DateTimeFormatOptions = {}): string {
  if (!ts) return '—'
  return new Intl.DateTimeFormat('en-GB', {
    timeZone: timezone,
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
    ...opts,
  }).format(new Date(ts * 1000))
}

export function timeOnly(ts: number): string {
  return dateTime(ts, { day: undefined, month: undefined, second: undefined })
}

export function axisTime(ts: number, rangeSeconds: number): string {
  const d = new Date(ts * 1000)
  if (rangeSeconds > 2 * 86400) {
    return new Intl.DateTimeFormat('en-GB', { timeZone: timezone, day: '2-digit', month: 'short' }).format(d)
  }
  return new Intl.DateTimeFormat('en-GB', { timeZone: timezone, hour: '2-digit', minute: '2-digit', hour12: false }).format(d)
}

export function shortId(id: string): string {
  return id.slice(0, 12)
}

export function imageTag(image: string): string {
  const at = image.indexOf('@')
  const base = at >= 0 ? image.slice(0, at) : image
  const slash = base.lastIndexOf('/')
  const colon = base.lastIndexOf(':')
  if (colon > slash) return base.slice(colon + 1)
  return 'latest'
}

export function imageName(image: string): string {
  const at = image.indexOf('@')
  const base = at >= 0 ? image.slice(0, at) : image
  const slash = base.lastIndexOf('/')
  const colon = base.lastIndexOf(':')
  return colon > slash ? base.slice(0, colon) : base
}

export function rangeSeconds(range: string): number {
  switch (range) {
    case '6h':
      return 6 * 3600
    case '24h':
      return 86400
    case '7d':
      return 7 * 86400
    case '30d':
      return 30 * 86400
    default:
      return 3600
  }
}
