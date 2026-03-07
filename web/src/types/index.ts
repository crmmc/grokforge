// API response wrapper types

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  page_size: number
  total_pages: number
}

// Re-export all types
export * from './token'
export * from './apikey'
export * from './system'
export * from './dashboard'
export * from './usage-log'
