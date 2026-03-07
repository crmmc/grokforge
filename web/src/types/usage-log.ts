// Usage log entry type matching GET /admin/usage/logs response

export interface UsageLogEntry {
  id: number
  api_key_name: string
  model: string
  ttft_ms: number
  duration_ms: number
  tokens_input: number
  tokens_output: number
  cache_tokens: number
  status: number
  estimated: boolean
  created_at: string
}
