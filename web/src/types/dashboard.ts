// Dashboard stats types mirroring Go API responses

export interface DashboardTokenStats {
  total: number
  active: number
  exhausted: number
  expired: number
  disabled: number
}

export interface ModeQuotaSummary {
  total_remaining: number
  total_limit: number
}

export interface PoolQuota {
  pool: string
  token_count: number
  active_count: number
  disabled_count: number
  expired_count: number
  mode_quotas: Record<string, ModeQuotaSummary>
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
