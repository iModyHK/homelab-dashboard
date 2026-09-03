import { Area, AreaChart, CartesianGrid, Legend, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { axisTime, dateTime } from '../lib/format'
import { chart, type Theme } from '../lib/theme'

export interface Series {
  key: string
  label: string
  color?: number
}

interface Props {
  data: ReadonlyArray<object>
  series: Series[]
  rangeSeconds: number
  theme: Theme
  format: (v: number) => string
  height?: number
  yMax?: number
  stacked?: boolean
}

interface TipProps {
  active?: boolean
  payload?: ReadonlyArray<{ value?: unknown; name?: unknown; color?: string }>
  label?: unknown
}

export function SeriesChart({ data, series, rangeSeconds, theme, format, height = 180, yMax, stacked = false }: Props) {
  const palette = chart[theme]
  const Tip = ({ active, payload, label }: TipProps) => {
    if (!active || !payload?.length) return null
    return (
      <div className="rounded-md border border-zinc-200 bg-white/95 px-2.5 py-1.5 text-xs shadow-md dark:border-zinc-700 dark:bg-zinc-900/95">
        <div className="mb-1 text-zinc-500">{dateTime(Number(label))}</div>
        {payload.map((p, i) => (
          <div key={i} className="flex items-center gap-2 tabular-nums">
            <span className="inline-block h-2 w-2 rounded-sm" style={{ background: p.color }} />
            <span className="text-zinc-600 dark:text-zinc-300">{String(p.name)}</span>
            <span className="ml-auto font-medium">{format(Number(p.value))}</span>
          </div>
        ))}
      </div>
    )
  }
  if (!data.length) {
    return <div className="flex items-center justify-center text-sm text-zinc-500" style={{ height }}>No samples in this range yet</div>
  }
  return (
    <ResponsiveContainer width="100%" height={height}>
      <AreaChart data={data as object[]} margin={{ top: 8, right: 8, bottom: 0, left: 0 }}>
        <CartesianGrid stroke={palette.grid} strokeDasharray="0" vertical={false} />
        <XAxis
          dataKey="ts"
          type="number"
          domain={['dataMin', 'dataMax']}
          tickFormatter={(v: number) => axisTime(v, rangeSeconds)}
          tick={{ fill: palette.axis, fontSize: 11 }}
          axisLine={{ stroke: palette.grid }}
          tickLine={false}
          minTickGap={40}
        />
        <YAxis
          width={52}
          domain={[0, yMax ?? 'auto']}
          tickFormatter={(v: number) => format(v)}
          tick={{ fill: palette.axis, fontSize: 11 }}
          axisLine={false}
          tickLine={false}
        />
        <Tooltip content={(p) => <Tip {...(p as TipProps)} />} cursor={{ stroke: palette.axis, strokeWidth: 1 }} />
        {series.length > 1 && <Legend iconType="plainline" wrapperStyle={{ fontSize: 11, color: palette.axis }} />}
        {series.map((s, i) => {
          const color = palette.series[s.color ?? i % palette.series.length]
          return (
            <Area
              key={s.key}
              type="monotone"
              dataKey={s.key}
              name={s.label}
              stroke={color}
              strokeWidth={2}
              fill={color}
              fillOpacity={0.12}
              stackId={stacked ? 'a' : undefined}
              isAnimationActive={false}
              dot={false}
              activeDot={{ r: 4, strokeWidth: 2, stroke: palette.surface }}
            />
          )
        })}
      </AreaChart>
    </ResponsiveContainer>
  )
}
