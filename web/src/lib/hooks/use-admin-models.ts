import { useQuery } from '@tanstack/react-query'
import { api } from '../api-client'

export interface AdminModelEntry {
  id: string
  display_name: string
  type: string
  public_type: string
  pool_floor: string
  quota_mode: string
  upstream_model?: string
  upstream_mode?: string
  force_thinking?: boolean
  enable_pro?: boolean
  enabled: boolean
}

export const adminModelKeys = {
  all: ['admin', 'models'] as const,
}

export function useAdminModels() {
  return useQuery({
    queryKey: adminModelKeys.all,
    queryFn: () => api.get<AdminModelEntry[]>('/models'),
    staleTime: 5 * 60 * 1000,
  })
}
