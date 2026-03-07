'use client'

import { useState, useEffect, FormEvent } from 'react'
import { useRouter } from 'next/navigation'
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { useToast } from '@/components/ui/toaster'
import { useTranslation } from '@/lib/i18n/context'
import { loginWithKey, verifySession } from '@/lib/auth'

export default function LoginPage() {
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [checking, setChecking] = useState(true)
  const router = useRouter()
  const { toast } = useToast()
  const { t } = useTranslation()

  // Auto-verify existing session cookie on mount
  useEffect(() => {
    verifySession()
      .then((valid) => {
        if (valid) {
          router.replace('/tokens/')
        } else {
          setChecking(false)
        }
      })
      .catch(() => setChecking(false))
  }, [router])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!password.trim() || loading) return

    setLoading(true)
    try {
      const valid = await loginWithKey(password.trim())
      if (valid) {
        router.replace('/tokens/')
      } else {
        toast({ title: t.login.invalidKey, variant: 'destructive' })
      }
    } catch {
      toast({ title: t.login.connectionFailed, variant: 'destructive' })
    } finally {
      setLoading(false)
    }
  }

  if (checking) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="text-muted">{t.common.loading}</div>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <form onSubmit={handleSubmit}>
        <Card className="w-[380px]">
          <CardHeader className="text-center">
            <CardTitle className="text-2xl">{t.login.title}</CardTitle>
            <CardDescription>{t.login.subtitle}</CardDescription>
          </CardHeader>
          <CardContent>
            <Input
              type="password"
              placeholder={t.login.placeholder}
              aria-label={t.login.placeholder}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoFocus
            />
          </CardContent>
          <CardFooter>
            <Button type="submit" className="w-full" disabled={loading || !password.trim()}>
              {loading ? t.common.loading : t.login.button}
            </Button>
          </CardFooter>
        </Card>
      </form>
    </div>
  )
}
