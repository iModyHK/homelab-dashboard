export function Sparkline({ values, width = 72, height = 20, max }: { values: number[]; width?: number; height?: number; max?: number }) {
  if (!values || values.length < 2) {
    return <svg width={width} height={height} className="text-zinc-300 dark:text-zinc-700" aria-hidden="true" />
  }
  const top = max ?? Math.max(1, ...values)
  const step = width / (values.length - 1)
  const pts = values.map((v, i) => `${(i * step).toFixed(1)},${(height - (Math.min(v, top) / top) * (height - 2) - 1).toFixed(1)}`)
  const path = `M${pts.join(' L')}`
  const area = `${path} L${width},${height} L0,${height} Z`
  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} className="text-sky-500" aria-hidden="true">
      <path d={area} fill="currentColor" opacity={0.15} />
      <path d={path} fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  )
}
