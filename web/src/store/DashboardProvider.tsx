import { createContext, useCallback, useContext, useEffect, useMemo, useReducer, useRef, useState, type ReactNode } from 'react'
import { api, UNAUTHORIZED_EVENT } from '../lib/api'
import { setTimezone } from '../lib/format'
import type { AlertRecord, Container, EventRecord, LiveMessage, Overview } from '../lib/types'

interface State {
  overview: Overview | null
  loading: boolean
  error: string
  connected: boolean
  errorCount: number
}

type Action =
  | { type: 'loaded'; overview: Overview }
  | { type: 'failed'; error: string }
  | { type: 'connection'; connected: boolean }
  | { type: 'live'; message: LiveMessage }

const MAX_EVENTS = 100
const MAX_SPARK = 30

function reduce(state: State, action: Action): State {
  switch (action.type) {
    case 'loaded':
      return { ...state, overview: action.overview, loading: false, error: '' }
    case 'failed':
      return { ...state, loading: false, error: action.error }
    case 'connection':
      return { ...state, connected: action.connected }
    case 'live':
      return applyLive(state, action.message)
  }
}

function applyLive(state: State, msg: LiveMessage): State {
  const ov = state.overview
  if (!ov) return state
  switch (msg.type) {
    case 'stats': {
      const containers = ov.containers.map((c) => {
        const live = msg.data.containers[c.id]
        if (!live) return c
        const spark = [...(c.sparkline ?? []), live.cpu].slice(-MAX_SPARK)
        return { ...c, live, sparkline: spark }
      })
      return { ...state, overview: { ...ov, containers, stacks: msg.data.stacks ?? ov.stacks } }
    }
    case 'host':
      return { ...state, overview: { ...ov, host: msg.data } }
    case 'containers': {
      const previous = new Map(ov.containers.map((c) => [c.id, c]))
      const containers: Container[] = msg.data.containers.map((c) => {
        const old = previous.get(c.id)
        return { ...c, sparkline: c.sparkline?.length ? c.sparkline.slice(-MAX_SPARK) : (old?.sparkline ?? []) }
      })
      return { ...state, overview: { ...ov, containers, stacks: msg.data.stacks ?? ov.stacks } }
    }
    case 'event': {
      const events: EventRecord[] = [msg.data, ...ov.events.filter((e) => e.id !== msg.data.id)].slice(0, MAX_EVENTS)
      return { ...state, overview: { ...ov, events } }
    }
    case 'alert': {
      const a = msg.data
      let alerts: AlertRecord[]
      if (a.state === 'resolved') {
        alerts = ov.alerts.filter((x) => x.id !== a.id)
      } else {
        alerts = [a, ...ov.alerts.filter((x) => x.id !== a.id)]
      }
      return { ...state, overview: { ...ov, alerts } }
    }
    case 'alert_ack': {
      const alerts = ov.alerts.map((a) => (a.id === msg.data.id ? { ...a, ackedAt: msg.data.ackedAt } : a))
      return { ...state, overview: { ...ov, alerts } }
    }
    case 'disks':
      return { ...state, overview: { ...ov, disks: msg.data.disks ?? [], arrays: msg.data.arrays ?? [] } }
    case 'error':
      return { ...state, errorCount: state.errorCount + 1 }
    case 'errors':
      return { ...state, errorCount: state.errorCount + msg.data.count }
    default:
      return state
  }
}

interface Ctx extends State {
  refresh: () => Promise<void>
  resetErrorCount: () => void
  authenticated: boolean | null
  setAuthenticated: (v: boolean) => void
  now: number
}

const DashboardContext = createContext<Ctx | null>(null)

export function DashboardProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reduce, { overview: null, loading: true, error: '', connected: false, errorCount: 0 })
  const [authenticated, setAuthenticated] = useState<boolean | null>(null)
  const [now, setNow] = useState(() => Date.now() / 1000)
  const socket = useRef<WebSocket | null>(null)
  const errorBase = useRef(0)

  const refresh = useCallback(async () => {
    try {
      const overview = await api.get<Overview>('/api/overview')
      setTimezone(overview.timezone)
      dispatch({ type: 'loaded', overview })
    } catch (err) {
      dispatch({ type: 'failed', error: err instanceof Error ? err.message : 'failed to load' })
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    api
      .get<{ authenticated: boolean }>('/api/auth/session')
      .then((r) => !cancelled && setAuthenticated(r.authenticated))
      .catch(() => !cancelled && setAuthenticated(false))
    const onUnauthorized = () => setAuthenticated(false)
    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
    return () => {
      cancelled = true
      window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized)
    }
  }, [])

  useEffect(() => {
    const t = setInterval(() => setNow(Date.now() / 1000), 1000)
    return () => clearInterval(t)
  }, [])

  useEffect(() => {
    if (!authenticated) return
    let closed = false
    let attempt = 0
    let timer: ReturnType<typeof setTimeout> | undefined
    void refresh()

    const connect = () => {
      if (closed) return
      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      const ws = new WebSocket(`${proto}://${location.host}/api/live`)
      socket.current = ws
      ws.onopen = () => {
        attempt = 0
        dispatch({ type: 'connection', connected: true })
        void refresh()
      }
      ws.onmessage = (ev) => {
        try {
          dispatch({ type: 'live', message: JSON.parse(ev.data as string) as LiveMessage })
        } catch {
          /* ignore malformed frame */
        }
      }
      ws.onclose = () => {
        dispatch({ type: 'connection', connected: false })
        if (closed) return
        attempt++
        timer = setTimeout(connect, Math.min(30000, 1000 * 2 ** Math.min(attempt, 5)))
      }
      ws.onerror = () => ws.close()
    }
    connect()
    return () => {
      closed = true
      if (timer) clearTimeout(timer)
      socket.current?.close()
    }
  }, [authenticated, refresh])

  const resetErrorCount = useCallback(() => {
    errorBase.current = state.errorCount
  }, [state.errorCount])

  const value = useMemo<Ctx>(
    () => ({
      ...state,
      errorCount: state.errorCount - errorBase.current,
      refresh,
      resetErrorCount,
      authenticated,
      setAuthenticated,
      now,
    }),
    [state, refresh, resetErrorCount, authenticated, now],
  )

  return <DashboardContext.Provider value={value}>{children}</DashboardContext.Provider>
}

export function useDashboard(): Ctx {
  const ctx = useContext(DashboardContext)
  if (!ctx) throw new Error('useDashboard outside provider')
  return ctx
}
