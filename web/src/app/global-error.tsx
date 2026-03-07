'use client'

import './globals.css'
import { ErrorFallback } from '@/components/layout/error-fallback'

export default function GlobalError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <html lang="zh">
      <body>
        <ErrorFallback error={error} reset={reset} />
      </body>
    </html>
  )
}
