import { useEffect, useState } from 'react'
import { RULE_TITLES } from '../components/AlertBanner'
import { Badge, Button, Empty, Panel, Spinner } from '../components/ui'
import { api, ApiError } from '../lib/api'
import { dateTime, relative } from '../lib/format'
import type { AlertRecord } from '../lib/types'
import { useDashboard } from '../store/DashboardProvider'

export function Alerts() {
  const { overview, now } = useDashboard()
  const [history, setHistory] = useState<AlertRecord[] | null>(null)
  const [testState, setTestState] = useState<'idle' | 'sending' | 'sent' | 'failed'>('idle')
  const [testError, setTestError] = useState('')

  const sendTest = async () => {
    setTestState('sending')
    setTestError('')
    try {
      await api.post('/api/alerts/test')
      setTestState('sent')
    } catch (err) {
      setTestState('failed')
      setTestError(err instanceof ApiError ? err.message : 'failed')
    }
    setTimeout(() => setTestState('idle'), 4000)
  }

  const load = () => api.get<AlertRecord[]>('/api/alerts?limit=200').then(setHistory).catch(() => undefined)
  useEffect(() => {
    void load()
  }, [overview?.alerts])

  const ack = async (id: number) => {
    await api.post(`/api/alerts/${id}/ack`).catch(() => undefined)
    void load()
  }

  if (!history) {
    return (
      <div className="flex justify-center p-12">
        <Spinner />
      </div>
    )
  }
  const firing = history.filter((a) => a.state === 'firing')
  const resolved = history.filter((a) => a.state !== 'firing')

  const row = (a: AlertRecord) => (
    <div key={a.id} className="flex flex-wrap items-center gap-2 border-b border-zinc-100 py-2 text-sm last:border-0 dark:border-zinc-800/60">
      <Badge tone={a.state !== 'firing' ? 'good' : a.severity === 'critical' ? 'critical' : 'warning'}>{a.state !== 'firing' ? 'resolved' : a.severity}</Badge>
      <span className="font-medium">{RULE_TITLES[a.rule] ?? a.rule}</span>
      <span className="text-zinc-700 dark:text-zinc-300">{a.targetName}</span>
      <span className="text-zinc-500">{a.message}</span>
      <span className="ml-auto flex items-center gap-2 text-xs tabular-nums text-zinc-500">
        <span title={dateTime(a.firedAt)}>fired {relative(a.firedAt, now)}</span>
        {a.resolvedAt > 0 && <span title={dateTime(a.resolvedAt)}>resolved {relative(a.resolvedAt, now)}</span>}
        {a.notifiedAt > 0 && <span title={`sent ${dateTime(a.notifiedAt)}`}>✓ sent</span>}
        {a.state === 'firing' && (a.ackedAt > 0 ? <span>acked</span> : <Button small onClick={() => void ack(a.id)}>Acknowledge</Button>)}
      </span>
    </div>
  )

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-xl font-semibold tracking-tight">Alerts</h1>
        <span className="ml-auto flex items-center gap-2">
          {testError && <span className="text-xs text-red-600 dark:text-red-400">{testError}</span>}
          <Button small onClick={() => void sendTest()} disabled={testState === 'sending'}>
            {testState === 'sending' ? 'Sending…' : testState === 'sent' ? 'Sent to Telegram' : testState === 'failed' ? 'Failed' : 'Send test alert'}
          </Button>
        </span>
      </div>
      <Panel title={`Active (${firing.length})`}>{firing.length ? firing.map(row) : <Empty>No active alerts.</Empty>}</Panel>
      <Panel title={`History (${resolved.length})`}>{resolved.length ? resolved.map(row) : <Empty>No resolved alerts yet.</Empty>}</Panel>
    </div>
  )
}
