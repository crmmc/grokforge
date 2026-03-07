import { useQuery } from '@tanstack/react-query'
import { api } from '../api-client'
import type { DashboardTokenStats, QuotaStatsResponse, UsageStatsResponse } from '@/types'

export const dashboardKeys = {
  tokens: ['dashboard', 'tokens'] as const,
  quota: ['dashboard', 'quota'] as const,
  usage: ['dashboard', 'usage'] as const,
}

export function useDashboardTokenStats() {
  return useQuery({
    queryKey: dashboardKeys.tokens,
    queryFn: () => api.get<DashboardTokenStats>('/stats/tokens'),
    refetchInterval: 30000,
  })
}

export function useQuotaStats() {
  return useQuery({
    queryKey: dashboardKeys.quota,
    queryFn: () => api.get<QuotaStatsResponse>('/stats/quota'),
    refetchInterval: 30000,
  })
}

export function useDashboardUsageStats() {
  return useQuery({
    queryKey: dashboardKeys.usage,
    queryFn: () => api.get<UsageStatsResponse>('/stats/usage'),
    refetchInterval: 30000,
  })
}
