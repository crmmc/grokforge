export interface CacheTypeStats {
  count: number
  size_mb: number
  limit_mb: number
}

export interface CacheStatsResponse {
  image: CacheTypeStats
  video: CacheTypeStats
}

export interface CacheFile {
  name: string
  size_bytes: number
  mod_time_ms: number
}

export interface CacheFileListResponse {
  total: number
  page: number
  page_size: number
  items: CacheFile[]
}

export interface CacheBatchResult {
  success: number
  failed: number
}

export interface CacheClearResult {
  deleted: number
  freed_mb: number
}

export type CacheMediaType = 'video'
