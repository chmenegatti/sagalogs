import { useEffect, useState } from 'react'

export type Theme = 'light' | 'dark'
const KEY = 'sagalogs-theme'

/** Tema salvo pelo usuário; sem escolha, segue o sistema. O index.html aplica antes do primeiro paint. */
function initial(): Theme {
  try {
    const saved = localStorage.getItem(KEY)
    if (saved === 'light' || saved === 'dark') return saved
  } catch {
    /* storage bloqueado: segue o sistema */
  }
  return matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function useTheme() {
  const [theme, setTheme] = useState<Theme>(initial)
  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark')
  }, [theme])
  const toggle = () =>
    setTheme((t) => {
      const next = t === 'dark' ? 'light' : 'dark'
      try {
        localStorage.setItem(KEY, next)
      } catch {
        /* sem persistência: vale só nesta aba */
      }
      return next
    })
  return { theme, toggle }
}
