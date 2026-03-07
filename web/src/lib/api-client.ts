// API client configuration and base fetch wrapper

const API_BASE = '/admin'
let isRedirectingToLogin = false

export class APIError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string
  ) {
    super(message)
    this.name = 'APIError'
  }
}

interface FetchOptions extends RequestInit {
  params?: Record<string, string | number | boolean | undefined>
}

function redirectToLogin() {
  if (typeof window === 'undefined' || isRedirectingToLogin) {
    return
  }

  if (window.location.pathname.startsWith('/login')) {
    return
  }

  isRedirectingToLogin = true
  window.location.replace('/login/')
}

export async function apiFetch<T>(endpoint: string, options: FetchOptions = {}): Promise<T> {
  const { params, ...fetchOptions } = options

  let url = `${API_BASE}${endpoint}`
  if (params) {
    const searchParams = new URLSearchParams()
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined) {
        searchParams.append(key, String(value))
      }
    })
    const queryString = searchParams.toString()
    if (queryString) {
      url += `?${queryString}`
    }
  }

  const headers = new Headers(fetchOptions.headers)
  if (fetchOptions.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(url, {
    ...fetchOptions,
    headers,
  })

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Unknown error' }))
    if (response.status === 401) {
      redirectToLogin()
    }
    throw new APIError(response.status, error.code || 'UNKNOWN', error.error || 'Request failed')
  }

  if (response.status === 204) {
    return undefined as T
  }

  return response.json()
}

export const api = {
  get: <T>(endpoint: string, params?: Record<string, string | number | boolean | undefined>) =>
    apiFetch<T>(endpoint, { method: 'GET', params }),

  post: <T>(endpoint: string, data?: unknown) =>
    apiFetch<T>(endpoint, { method: 'POST', body: data ? JSON.stringify(data) : undefined }),

  put: <T>(endpoint: string, data?: unknown) =>
    apiFetch<T>(endpoint, { method: 'PUT', body: data ? JSON.stringify(data) : undefined }),

  patch: <T>(endpoint: string, data?: unknown) =>
    apiFetch<T>(endpoint, { method: 'PATCH', body: data ? JSON.stringify(data) : undefined }),

  delete: <T>(endpoint: string) =>
    apiFetch<T>(endpoint, { method: 'DELETE' }),
}
