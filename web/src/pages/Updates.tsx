import { useEffect, useState } from 'react'
import { Badge, Button, Empty, Panel, Spinner } from '../components/ui'
import { api } from '../lib/api'
import { relative } from '../lib/format'
import type { ImageUpdate, SourceStatus } from '../lib/types'
import { useDashboard } from '../store/DashboardProvider'

export function Updates() {
  const { now } = useDashboard()
  const [data, setData] = useState<{ images: ImageUpdate[]; registry?: SourceStatus } | null>(null)
  const [checking, setChecking] = useState(false)

  const load = () => api.get<{ images: ImageUpdate[]; registry?: SourceStatus }>('/api/updates').then(setData).catch(() => undefined)

  useEffect(() => {
    void load()
    const t = setInterval(() => void load(), 30000)
    return () => clearInterval(t)
  }, [])

  const check = async () => {
    setChecking(true)
    try {
      await api.post('/api/updates/check')
      setTimeout(() => {
        void load()
        setChecking(false)
      }, 8000)
    } catch {
      setChecking(false)
    }
  }

  if (!data) {
    return (
      <div className="flex justify-center p-12">
        <Spinner />
      </div>
    )
  }
  const available = data.images.filter((i) => i.updateAvailable)
  const current = data.images.filter((i) => !i.updateAvailable && !i.error)
  const skipped = data.images.filter((i) => i.error)
  const checked = data.images.reduce((m, i) => Math.max(m, i.checkedAt), 0)

  const row = (i: ImageUpdate) => (
    <div key={i.image} className="flex flex-wrap items-center gap-2 border-b border-zinc-100 py-2 text-sm last:border-0 dark:border-zinc-800/60">
      <span className="font-mono text-xs">{i.image}</span>
      <span className="text-xs text-zinc-500">{i.containers.join(', ')}</span>
      <span className="ml-auto flex items-center gap-2">
        {i.updateAvailable && <Badge tone="info">newer digest upstream</Badge>}
        {i.error && <Badge tone="neutral">{i.error}</Badge>}
        {i.localDigest && <span className="font-mono text-[10px] text-zinc-400" title={`local ${i.localDigest}\nremote ${i.remoteDigest}`}>{i.localDigest.slice(7, 19)}{i.updateAvailable ? ` → ${i.remoteDigest.slice(7, 19)}` : ''}</span>}
      </span>
    </div>
  )

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-xl font-semibold tracking-tight">Image updates</h1>
        <span className="text-sm text-zinc-500">{checked ? `checked ${relative(checked, now)}` : 'not checked yet'}</span>
        {data.registry && !data.registry.ok && <span className="text-xs text-amber-700 dark:text-amber-300">{data.registry.lastError}</span>}
        <span className="ml-auto">
          <Button small onClick={() => void check()} disabled={checking}>
            {checking ? 'Checking…' : 'Check now'}
          </Button>
        </span>
      </div>
      <Panel title={`Updates available (${available.length})`}>{available.length ? available.map(row) : <Empty>Everything running matches its upstream tag.</Empty>}</Panel>
      <Panel title={`Up to date (${current.length})`}>{current.length ? current.map(row) : <Empty>No images checked yet.</Empty>}</Panel>
      {skipped.length > 0 && <Panel title={`Not checked (${skipped.length})`}>{skipped.map(row)}</Panel>}
    </div>
  )
}
