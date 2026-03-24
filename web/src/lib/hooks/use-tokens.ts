import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api-client'
import type {
  Token,
  TokenUpdateRequest,
  PaginatedResponse,
} from '@/types'

export const tokenKeys = {
  all: ['tokens'] as const,
  lists: () => [...tokenKeys.all, 'list'] as const,
  list: (params: { page?: number; page_size?: number; status?: string; nsfw?: boolean }) => [...tokenKeys.lists(), params] as const,
  idsByStatus: (status: string | null) => [...tokenKeys.all, 'ids', status] as const,
  details: () => [...tokenKeys.all, 'detail'] as const,
  detail: (id: number) => [...tokenKeys.details(), id] as const,
  stats: () => [...tokenKeys.all, 'stats'] as const,
}

export function useTokens(params: { page?: number; page_size?: number; status?: string; nsfw?: boolean } = {}) {
  return useQuery({
    queryKey: tokenKeys.list(params),
    queryFn: () => api.get<PaginatedResponse<Token>>('/tokens', params),
  })
}

export function useToken(id: number | null) {
  return useQuery({
    queryKey: tokenKeys.detail(id ?? 0),
    queryFn: async () => {
      if (id === null) {
        throw new Error('token id is required')
      }
      return api.get<Token>(`/tokens/${id}`)
    },
    enabled: id !== null,
  })
}

export function useUpdateToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: TokenUpdateRequest }) =>
      api.put<Token>(`/tokens/${id}`, data),
    onSuccess: (_, { id }) => {
      queryClient.invalidateQueries({ queryKey: tokenKeys.detail(id) })
      queryClient.invalidateQueries({ queryKey: tokenKeys.lists() })
    },
  })
}

export function useDeleteToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.delete<void>(`/tokens/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: tokenKeys.all })
    },
  })
}

export type BatchOperation = 'enable' | 'disable' | 'delete' | 'enable_nsfw' | 'disable_nsfw' | 'export' | 'import'

export interface BatchTokenRequest {
  operation: BatchOperation
  ids?: number[]
  tokens?: string[]
  pool?: string
  quota?: number
  chat_quota?: number
  image_quota?: number
  video_quota?: number
  priority?: number
  status?: string
  remark?: string
  nsfw_enabled?: boolean
  raw?: boolean
}

export interface BatchTokenResponse {
  operation: string
  success: number
  failed: number
  errors?: Array<{ index?: number; id?: number; message: string }>
  tokens?: Token[]
  raw_tokens?: string[]
}

export function useBatchTokens() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: BatchTokenRequest) => {
      const endpoint = req.raw ? '/tokens/batch?raw=true' : '/tokens/batch'
      const { raw: _, ...body } = req
      return api.post<BatchTokenResponse>(endpoint, body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: tokenKeys.all })
    },
  })
}

export function useRefreshToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.post<Token>(`/tokens/${id}/refresh`),
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({ queryKey: tokenKeys.detail(id) })
      queryClient.invalidateQueries({ queryKey: tokenKeys.lists() })
    },
  })
}

export function useTokenIdsByStatus(status: string | null) {
  return useQuery({
    queryKey: tokenKeys.idsByStatus(status),
    queryFn: () => api.get<{ ids: number[] }>('/tokens/ids', status ? { status } : {}),
    enabled: status !== null,
  })
}
