import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchModels, getApiKey } from '@/lib/function-api'

export const modelKeys = { all: ['models'] as const }

const imageModelIDs = new Set(['grok-imagine-1.0', 'grok-imagine-1.0-fast'])
const videoModelIDs = new Set(['grok-imagine-1.0-video'])

function useAllModels() {
  return useQuery({
    queryKey: modelKeys.all,
    queryFn: fetchModels,
    staleTime: 5 * 60 * 1000,
    enabled: !!getApiKey(),
  })
}

export function useImageModels() {
  const { data, isLoading, error } = useAllModels()
  const models = useMemo(
    () => data?.data.filter((m) => imageModelIDs.has(m.id)).map((m) => m.id) ?? [],
    [data]
  )
  return { models, isLoading, error }
}

export function useVideoModels() {
  const { data, isLoading, error } = useAllModels()
  const models = useMemo(
    () => data?.data.filter((m) => videoModelIDs.has(m.id)).map((m) => m.id) ?? [],
    [data]
  )
  return { models, isLoading, error }
}

export function useChatModels() {
  const { data, isLoading, error } = useAllModels()
  const models = useMemo(
    () =>
      data?.data
        .filter((m) => !m.id.startsWith('grok-imagine-') && !m.id.startsWith('grok-video-') && !m.id.includes('imageGen'))
        .map((m) => m.id) ?? [],
    [data]
  )
  return { models, isLoading, error }
}
