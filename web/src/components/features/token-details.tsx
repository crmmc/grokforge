'use client'

import { Progress } from '@/components/ui'
import { cn } from '@/lib/utils'
import { useTranslation } from '@/lib/i18n/context'
import type { Token } from '@/types'
import { buildTokenQuotaMetrics, quotaProgressColor, quotaSurfaceColor, quotaTextColor } from './token-quota-utils'

function formatTokenDate(value: string | null): string {
  if (!value) return '-'
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

export function TokenDetails({ token }: { token: Token }) {
  const { t } = useTranslation()
  const modeLabels: Record<string, string> = {
    chat: t.tokens.chatQuota,
    image: t.tokens.imageQuota,
    video: t.tokens.videoQuota,
  }
  const quotaMetrics = buildTokenQuotaMetrics(token, modeLabels)

  return (
    <div className="space-y-4">
      <div className="grid gap-3 md:grid-cols-3">
        {quotaMetrics.map((metric) => (
          <div key={metric.key} className={cn('rounded-lg border p-3', quotaSurfaceColor(metric.percent))}>
            <div className="flex items-start justify-between gap-3">
              <span className="text-sm text-muted">{metric.label}</span>
              <span className={cn('text-xs font-semibold', quotaTextColor(metric.percent))}>{metric.percent.toFixed(0)}%</span>
            </div>
            <div className="mt-1 text-base font-semibold">{metric.remaining} / {metric.total}</div>
            <Progress value={metric.percent} className={cn('mt-3 h-2', quotaProgressColor(metric.percent))} />
          </div>
        ))}
      </div>

      <div className="grid grid-cols-2 gap-4 text-sm md:grid-cols-4">
        <div>
          <span className="text-muted">{t.tokens.failCount}</span>
          <span className="ml-2 font-medium">{token.fail_count || 0}</span>
        </div>
        <div>
          <span className="text-muted">{t.tokens.lastUsed}</span>
          <span className="ml-2 font-medium">{formatTokenDate(token.last_used)}</span>
        </div>
        <div>
          <span className="text-muted">{t.tokens.nsfw}:</span>
          <span className="ml-2 font-medium">
            {token.nsfw_enabled ? t.common.enabled : t.common.disabled}
          </span>
        </div>
        <div className="md:col-span-2">
          <span className="text-muted">{t.tokens.remark}:</span>
          <span className="ml-2 font-medium">{token.remark || '-'}</span>
        </div>
        <div className="md:col-span-2">
          <span className="text-muted">{t.tokens.createdAt}</span>
          <span className="ml-2 font-medium">{formatTokenDate(token.created_at)}</span>
        </div>
      </div>
    </div>
  )
}
