'use client'

import { useState, useEffect } from 'react'
import { useRouter } from 'next/navigation'

/**
 * Login via POST /admin/login, server sets httpOnly cookie.
 */
export async function loginWithKey(key: string): Promise<boolean> {
  try {
    const resp = await fetch('/admin/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ key }),
    })
    return resp.ok
  } catch {
    return false
  }
}

/**
 * Verify existing session cookie via GET /admin/verify.
 * Cookie is sent automatically by the browser.
 */
export async function verifySession(): Promise<boolean> {
  try {
    const resp = await fetch('/admin/verify', { method: 'GET' })
    return resp.ok
  } catch {
    return false
  }
}

/**
 * Auth guard hook for admin pages.
 * Checks session cookie validity on mount, redirects to /login/ if invalid.
 */
export function useAuthGuard(): { isAuthenticated: boolean; isLoading: boolean } {
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const router = useRouter()

  useEffect(() => {
    verifySession()
      .then((valid) => {
        if (valid) {
          setIsAuthenticated(true)
        } else {
          router.replace('/login/')
        }
      })
      .finally(() => setIsLoading(false))
  }, [router])

  return { isAuthenticated, isLoading }
}
