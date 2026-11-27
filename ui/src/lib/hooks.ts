import { useQuery } from '@tanstack/react-query'
import { useEffect } from 'react'
import { type Container, api } from './api'
import { registerServices } from './meta'

const NONE: Container[] = []

export function useContainers() {
  const q = useQuery({ queryKey: ['containers'], queryFn: api.containers, refetchInterval: 3000 })
  useEffect(() => {
    if (q.data) registerServices(q.data.map((c) => c.name))
  }, [q.data])
  return q.data ?? NONE
}
