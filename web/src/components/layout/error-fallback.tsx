'use client'

import { useEffect, useMemo } from 'react'
import { AlertTriangle, RefreshCw, RotateCcw } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'

type Locale = 'zh' | 'en'

interface ErrorFallbackProps {
  error?: Error & { digest?: string }
  reset?: () => void
}

const MESSAGES = {
  zh: {
    badge: '系统异常',
    title: '页面暂时无法显示',
    description: '界面遇到了未处理错误。你可以先重试当前页面；如果仍然失败，再刷新页面或重新登录。',
    retry: '重试',
    reload: '刷新页面',
    reference: '错误标识',
  },
  en: {
    badge: 'System Error',
    title: 'This page is temporarily unavailable',
    description: 'The interface hit an unexpected error. Try rendering the page again first, then reload the page or sign in again if it keeps failing.',
    retry: 'Try again',
    reload: 'Reload page',
    reference: 'Error reference',
  },
} as const

function getLocale(): Locale {
  if (typeof document !== 'undefined' && document.documentElement.lang.toLowerCase().startsWith('zh')) {
    return 'zh'
  }

  if (typeof navigator !== 'undefined' && navigator.language.toLowerCase().startsWith('zh')) {
    return 'zh'
  }

  return 'en'
}

export function ErrorFallback({ error, reset }: ErrorFallbackProps) {
  const locale = useMemo(() => getLocale(), [])
  const t = MESSAGES[locale]

  useEffect(() => {
    if (error) {
      console.error(error)
    }
  }, [error])

  return (
    <div className="flex min-h-screen items-center justify-center px-4 py-10 bg-background text-foreground">
      <Card className="w-full max-w-xl border-destructive/20 shadow-[0_24px_80px_rgba(0,0,0,0.08)]">
        <CardHeader className="space-y-4">
          <div className="inline-flex w-fit items-center gap-2 rounded-full border border-destructive/15 bg-destructive/5 px-3 py-1 text-xs font-medium text-destructive">
            <AlertTriangle className="h-4 w-4" />
            {t.badge}
          </div>
          <div className="space-y-2">
            <CardTitle className="text-xl sm:text-2xl">{t.title}</CardTitle>
            <CardDescription className="max-w-prose text-sm leading-6">{t.description}</CardDescription>
          </div>
        </CardHeader>
        {(error?.digest || error?.message) && (
          <CardContent>
            <div className="rounded-md border border-border/80 bg-muted/40 px-3 py-2 text-xs text-muted">
              <span className="font-medium text-foreground">{t.reference}:</span>{' '}
              {error?.digest || error?.message}
            </div>
          </CardContent>
        )}
        <CardFooter className="flex flex-col items-stretch gap-3 sm:flex-row sm:justify-end">
          {reset && (
            <Button type="button" onClick={reset}>
              <RotateCcw className="h-4 w-4" />
              {t.retry}
            </Button>
          )}
          <Button type="button" variant="outline" onClick={() => window.location.reload()}>
            <RefreshCw className="h-4 w-4" />
            {t.reload}
          </Button>
        </CardFooter>
      </Card>
    </div>
  )
}
