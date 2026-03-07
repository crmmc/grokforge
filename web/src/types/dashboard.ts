// Dashboard stats types mirroring Go API responses

export interface DashboardTokenStats {
  total: number
  active: number
  cooling: number
  expired: number
  disabled: number
}

export interface PoolQuota {
  pool: string
  total_chat_quota: number
  remaining_chat_quota: number
  total_image_quota: number
  remaining_image_quota: number
  total_video_quota: number
  remaining_video_quota: number
}

export interface QuotaStatsResponse {
  pools: PoolQuota[]
}

export interface HourlyUsage {
  hour: string
  endpoint: string
  count: number
}

export interface TokenTotals {
  input: number
  output: number
  cache: number
  total: number
}

export interface UsageStatsResponse {
  today: Record<string, number>
  total: number
  hourly: HourlyUsage[]
  delta: Record<string, number | null>
  tokens_today?: TokenTotals
}
