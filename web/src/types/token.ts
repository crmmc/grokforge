// Token types mirroring Go models

export interface Token {
  id: number
  token: string
  pool: string
  status: TokenStatus
  status_reason?: string
  chat_quota: number
  total_chat_quota: number
  image_quota: number
  total_image_quota: number
  video_quota: number
  total_video_quota: number
  fail_count: number
  cool_until: string | null
  last_used: string | null
  priority: number
  remark?: string
  nsfw_enabled?: boolean
  created_at: string
  updated_at: string
}

export type TokenStatus = 'active' | 'disabled' | 'expired' | 'cooling'

export interface TokenUpdateRequest {
  status?: TokenStatus
  pool?: string
  chat_quota?: number
  image_quota?: number
  video_quota?: number
  remark?: string
  nsfw_enabled?: boolean
}
