import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../api-client'
import type { SystemStatus, UsageStats, ConfigResponse } from '@/types'

export const systemKeys = {
  status: ['system', 'status'] as const,
  config: ['system', 'config'] as const,
  fullConfig: ['config'] as const,
  usage: (period: string) => ['system', 'usage', period] as const,
}

export function useSystemStatus() {
  return useQuery({
    queryKey: systemKeys.status,
    queryFn: () => api.get<SystemStatus>('/system/status'),
    refetchInterval: 30000, // Refresh every 30s
  })
}

export function useUsageStats(period: 'hour' | 'day' | 'week' | 'month' = 'day') {
  return useQuery({
    queryKey: systemKeys.usage(period),
    queryFn: () => api.get<UsageStats>('/system/usage', { period }),
  })
}

export function useConfig() {
  return useQuery({
    queryKey: systemKeys.fullConfig,
    queryFn: () => api.get<ConfigResponse>('/config'),
  })
}

export function useUpdateConfig() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<ConfigResponse>) => api.put<ConfigResponse>('/config', data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: systemKeys.fullConfig })
      queryClient.invalidateQueries({ queryKey: systemKeys.config })
    },
  })
}
