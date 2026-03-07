import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api-client'
import type {
  APIKey,
  APIKeyCreateRequest,
  APIKeyUpdateRequest,
  APIKeyCreateResponse,
  APIKeyStats,
  PaginatedResponse,
} from '@/types'

export const apiKeyKeys = {
  all: ['apikeys'] as const,
  lists: () => [...apiKeyKeys.all, 'list'] as const,
  list: (params: { page?: number; status?: string }) => [...apiKeyKeys.lists(), params] as const,
  details: () => [...apiKeyKeys.all, 'detail'] as const,
  detail: (id: number) => [...apiKeyKeys.details(), id] as const,
  stats: () => [...apiKeyKeys.all, 'stats'] as const,
}

export function useAPIKeys(params: { page?: number; page_size?: number; status?: string } = {}) {
  return useQuery({
    queryKey: apiKeyKeys.list(params),
    queryFn: () => api.get<PaginatedResponse<APIKey>>('/apikeys', params),
  })
}

export function useAPIKeyStats() {
  return useQuery({
    queryKey: apiKeyKeys.stats(),
    queryFn: () => api.get<APIKeyStats>('/apikeys/stats'),
  })
}

export function useCreateAPIKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: APIKeyCreateRequest) => api.post<APIKeyCreateResponse>('/apikeys', data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: apiKeyKeys.all })
    },
  })
}

export function useUpdateAPIKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: APIKeyUpdateRequest }) =>
      api.patch<APIKey>(`/apikeys/${id}`, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: apiKeyKeys.detail(id) })
      queryClient.invalidateQueries({ queryKey: apiKeyKeys.lists() })
    },
  })
}

export function useDeleteAPIKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.delete<void>(`/apikeys/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: apiKeyKeys.all })
    },
  })
}

export function useRegenerateAPIKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.post<APIKeyCreateResponse>(`/apikeys/${id}/regenerate`),
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({ queryKey: apiKeyKeys.detail(id) })
      queryClient.invalidateQueries({ queryKey: apiKeyKeys.lists() })
    },
  })
}
