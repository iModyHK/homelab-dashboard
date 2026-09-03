import { useCallback, useEffect, useState } from 'react'

export type Theme = 'dark' | 'light'

const KEY = 'hd-theme'

function stored(): Theme {
  try {
    const v = localStorage.getItem(KEY)
    if (v === 'light' || v === 'dark') return v
  } catch {
    /* storage unavailable */
  }
  return 'dark'
}

function apply(theme: Theme) {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  document.documentElement.style.colorScheme = theme
}

export function bootstrapTheme() {
  apply(stored())
}

export function useTheme(): [Theme, () => void] {
  const [theme, setTheme] = useState<Theme>(stored)
  useEffect(() => {
    apply(theme)
    try {
      localStorage.setItem(KEY, theme)
    } catch {
      /* storage unavailable */
    }
  }, [theme])
  const toggle = useCallback(() => setTheme((t) => (t === 'dark' ? 'light' : 'dark')), [])
  return [theme, toggle]
}

export const chart = {
  dark: {
    series: ['#3987e5', '#d95926', '#199e70', '#c98500', '#d55181', '#9085e9'],
    grid: '#2c2c2a',
    axis: '#898781',
    surface: '#1a1a19',
    ink: '#ffffff',
  },
  light: {
    series: ['#2a78d6', '#eb6834', '#1baf7a', '#eda100', '#e87ba4', '#4a3aa7'],
    grid: '#e1e0d9',
    axis: '#898781',
    surface: '#fcfcfb',
    ink: '#0b0b0b',
  },
}

export const status = {
  good: '#0ca30c',
  warning: '#fab219',
  serious: '#ec835a',
  critical: '#d03b3b',
}
