// Token types mirroring Go models

export interface Token {
  id: number
  token: string
  pool: string
  status: TokenStatus
  display_status: string // active | disabled | expired | exhausted
  status_reason?: string
  quotas: Record<string, number>       // mode -> remaining
  limit_quotas: Record<string, number> // mode -> upper limit
  fail_count: number
  last_used: string | null
  priority: number
  remark?: string
  nsfw_enabled?: boolean
  created_at: string
  updated_at: string
}

export type TokenStatus = 'active' | 'disabled' | 'expired' | 'exhausted'

export interface TokenUpdateRequest {
  status?: TokenStatus
  pool?: string
  quotas?: Record<string, number>
  remark?: string
  nsfw_enabled?: boolean
}
