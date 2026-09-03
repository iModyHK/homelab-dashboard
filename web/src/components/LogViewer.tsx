import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'
import { dateTime } from '../lib/format'
import type { LogLine } from '../lib/types'
import { Button, Segmented } from './ui'

const LEVELS = ['all', 'error', 'warn', 'info'] as const
type Level = (typeof LEVELS)[number]
const TAILS = ['200', '500', '2000'] as const

const levelClass: Record<string, string> = {
  error: 'text-red-600 dark:text-red-400',
  warn: 'text-amber-600 dark:text-amber-300',
  debug: 'text-zinc-400 dark:text-zinc-500',
  info: 'text-zinc-800 dark:text-zinc-200',
}

export function LogViewer({ containerId, running }: { containerId: string; running: boolean }) {
  const [lines, setLines] = useState<LogLine[]>([])
  const [level, setLevel] = useState<Level>('all')
  const [tail, setTail] = useState<(typeof TAILS)[number]>('500')
  const [query, setQuery] = useState('')
  const [applied, setApplied] = useState('')
  const [follow, setFollow] = useState(true)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [copied, setCopied] = useState(false)
  const box = useRef<HTMLDivElement>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const params = new URLSearchParams({ tail, level })
      if (applied) params.set('q', applied)
      const res = await api.get<{ lines: LogLine[] }>(`/api/containers/${containerId}/logs?${params}`)
      setLines(res.lines)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'failed to load logs')
    } finally {
      setLoading(false)
    }
  }, [containerId, tail, level, applied])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (!follow || !running) return
    const t = setInterval(() => void load(), 5000)
    return () => clearInterval(t)
  }, [follow, running, load])

  useEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight
  }, [lines, follow])

  const copy = async () => {
    const text = lines.map((l) => `${l.ts ? new Date(l.ts * 1000).toISOString() : ''} ${l.text}`).join('\n')
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      setError('clipboard unavailable')
    }
  }

  return (
    <div>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <Segmented value={level} options={LEVELS} onChange={setLevel} />
        <Segmented value={tail} options={TAILS} onChange={setTail} />
        <form
          className="flex items-center gap-1"
          onSubmit={(e) => {
            e.preventDefault()
            setApplied(query.trim())
          }}
        >
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="regex search"
            className="w-48 rounded-md border border-zinc-300 bg-white px-2 py-1 font-mono text-xs outline-none focus:border-zinc-500 dark:border-zinc-700 dark:bg-zinc-950"
          />
          <Button small type="submit">
            Search
          </Button>
          {applied && (
            <Button
              small
              onClick={() => {
                setQuery('')
                setApplied('')
              }}
            >
              Clear
            </Button>
          )}
        </form>
        <label className="flex items-center gap-1.5 text-xs text-zinc-600 dark:text-zinc-300">
          <input type="checkbox" checked={follow} onChange={(e) => setFollow(e.target.checked)} />
          follow
        </label>
        <span className="ml-auto flex items-center gap-2 text-xs text-zinc-500">
          {loading ? 'loading…' : `${lines.length} lines`}
          <Button small onClick={() => void load()}>
            Refresh
          </Button>
          <Button small onClick={() => void copy()}>
            {copied ? 'Copied' : 'Copy'}
          </Button>
        </span>
      </div>
      {error && <div className="mb-2 text-xs text-red-600 dark:text-red-400">{error}</div>}
      <div ref={box} className="h-[480px] overflow-auto rounded-md border border-zinc-200 bg-zinc-50 p-2 font-mono text-[12px] leading-5 dark:border-zinc-800 dark:bg-zinc-950">
        {lines.length === 0 ? (
          <div className="p-4 text-center text-zinc-500">No log lines.</div>
        ) : (
          lines.map((l, i) => (
            <div key={i} className="flex gap-2 whitespace-pre-wrap break-all">
              <span className="shrink-0 select-none text-zinc-400 dark:text-zinc-600">{l.ts ? dateTime(l.ts) : '—'}</span>
              <span className={`shrink-0 select-none ${l.stream === 'stderr' ? 'text-orange-500' : 'text-zinc-400 dark:text-zinc-600'}`}>{l.stream === 'stderr' ? 'E' : 'O'}</span>
              <span className={levelClass[l.level] ?? levelClass.info}>{l.text}</span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
