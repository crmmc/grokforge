// API Key types mirroring Go models

export interface APIKey {
  id: number
  key: string
  name: string
  status: APIKeyStatus
  model_whitelist: string[] | null
  rate_limit: number
  daily_limit: number
  daily_used: number
  total_used: number
  last_used_at: string | null
  expires_at: string | null
  created_at: string
  updated_at: string
}

export type APIKeyStatus = 'active' | 'inactive' | 'expired' | 'rate_limited'

export interface APIKeyCreateRequest {
  name: string
  model_whitelist?: string[]
  rate_limit?: number
  daily_limit?: number
  expires_at?: string
}

export interface APIKeyUpdateRequest {
  name?: string
  status?: APIKeyStatus
  model_whitelist?: string[]
  rate_limit?: number
  daily_limit?: number
  expires_at?: string
}

export interface APIKeyCreateResponse {
  id: number
  key: string
  name: string
}

export interface APIKeyStats {
  total: number
  active: number
  inactive: number
  expired: number
  rate_limited: number
}
