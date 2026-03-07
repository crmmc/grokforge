import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { api } from '../api-client'
import type { PaginatedResponse, UsageLogEntry } from '@/types'

export interface UsageLogParams {
  page: number
  pageSize: number
  sortBy?: string
  sortDir?: 'asc' | 'desc'
  model?: string
  period?: string
  status?: string
  apiKey?: string
}

export const usageLogKeys = {
  logs: (params: UsageLogParams) => ['usage', 'logs', params] as const,
}

export function useUsageLogs(params: UsageLogParams) {
  return useQuery({
    queryKey: usageLogKeys.logs(params),
    queryFn: () => api.get<PaginatedResponse<UsageLogEntry>>('/usage/logs', {
      page: params.page,
      page_size: params.pageSize,
      sort_by: params.sortBy,
      sort_dir: params.sortDir,
      model: params.model || undefined,
      period: params.period || undefined,
      status: params.status || undefined,
      api_key: params.apiKey || undefined,
    }),
    placeholderData: keepPreviousData,
  })
}
