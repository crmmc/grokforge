import { useQuery, useMutation, useQueryClient, keepPreviousData } from '@tanstack/react-query'
import { api } from '../api-client'
import type {
  CacheStatsResponse,
  CacheFileListResponse,
  CacheBatchResult,
  CacheClearResult,
  CacheMediaType,
} from '@/types/cache'

export const cacheKeys = {
  all: ['cache'] as const,
  stats: () => [...cacheKeys.all, 'stats'] as const,
  files: (type: CacheMediaType, page: number, pageSize: number) =>
    [...cacheKeys.all, 'files', type, page, pageSize] as const,
}

export function useCacheStats() {
  return useQuery({
    queryKey: cacheKeys.stats(),
    queryFn: () => api.get<CacheStatsResponse>('/cache/stats'),
  })
}

export function useCacheFiles(type: CacheMediaType, page: number = 1, pageSize: number = 50) {
  return useQuery({
    queryKey: cacheKeys.files(type, page, pageSize),
    queryFn: () =>
      api.get<CacheFileListResponse>('/cache/files', {
        type,
        page,
        page_size: pageSize,
      }),
    placeholderData: keepPreviousData,
  })
}

export function useDeleteCacheFiles() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ type, names }: { type: CacheMediaType; names: string[] }) =>
      api.post<CacheBatchResult>('/cache/delete', { type, names }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: cacheKeys.all })
    },
  })
}

export function useClearCache() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ type }: { type: CacheMediaType }) =>
      api.post<CacheClearResult>('/cache/clear', { type }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: cacheKeys.all })
    },
  })
}

/**
 * Get authenticated blob URL for a cache file (for preview/download).
 * Caller must call URL.revokeObjectURL when done.
 */
export async function fetchCacheFileBlob(type: CacheMediaType, name: string, download?: boolean): Promise<string> {
  const params = new URLSearchParams()
  if (download) params.set('download', 'true')
  const qs = params.toString()
  const url = `/admin/cache/files/${type}/${encodeURIComponent(name)}${qs ? '?' + qs : ''}`

  const response = await fetch(url)
  if (!response.ok) throw new Error(`Failed to fetch file: ${response.status}`)
  const blob = await response.blob()
  return URL.createObjectURL(blob)
}
