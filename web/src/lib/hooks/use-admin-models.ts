import { useQuery } from '@tanstack/react-query'
import { api } from '../api-client'

export interface AdminModelEntry {
  id: string
  display_name: string
  type: string
  public_type: string
  pool_floor: string
  mode?: string
  quota_sync: boolean
  cooldown_seconds?: number
  upstream_model?: string
  upstream_mode?: string
  force_thinking?: boolean
  enable_pro?: boolean
  enabled: boolean
}

export interface AdminModeGroup {
  mode: string
  display_name: string
  upstream_name: string
  window_seconds: number
  default_quotas: Record<string, number>
  models: string[]
}

export interface AdminModelsResponse {
  models: AdminModelEntry[]
  mode_groups: AdminModeGroup[]
}

export const adminModelKeys = {
  all: ['admin', 'models'] as const,
}

export function useAdminModels() {
  return useQuery({
    queryKey: adminModelKeys.all,
    queryFn: () => api.get<AdminModelsResponse>('/models'),
    staleTime: 5 * 60 * 1000,
  })
}
