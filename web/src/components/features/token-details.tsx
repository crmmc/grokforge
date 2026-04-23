'use client'

import { AlertCircle } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle, Badge, Progress } from '@/components/ui'
import { cn } from '@/lib/utils'
import { useTranslation } from '@/lib/i18n/context'
import type { QuotaPresentationGroup, QuotaPresentationIssue } from '@/lib/quota-presentation'
import { formatQuotaPresentationIssues, quotaProgressColor, quotaSurfaceColor, quotaTextColor } from '@/lib/quota-presentation'
import type { Token } from '@/types'

interface TokenDetailsProps {
  catalogError?: Error | null
  catalogLoading: boolean
  groups: QuotaPresentationGroup[]
  issues: QuotaPresentationIssue[]
  token: Token
}

function formatTokenDate(value: string | null): string {
  if (!value) return '-'
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

export function TokenDetails({ catalogError, catalogLoading, groups, issues, token }: TokenDetailsProps) {
  const { t } = useTranslation()
  const issueMessages = formatQuotaPresentationIssues(issues, t)

  return (
    <div className="space-y-4">
      {catalogError ? (
        <QuotaAlert title={t.common.humanReadableQuotaUnavailable} message={catalogError.message || t.common.unknownError} />
      ) : catalogLoading ? (
        <p className="text-sm text-muted">{t.common.loading}</p>
      ) : issues.length > 0 ? (
        <QuotaAlert title={t.common.humanReadableQuotaUnavailable} message={issueMessages.join(' / ')} />
      ) : null}

      {groups.length > 0 ? (
        <div className="grid gap-3 md:grid-cols-2">
          {groups.map((group) => (
            <div key={group.key} className={cn('rounded-lg border p-3', quotaSurfaceColor(group.percent))}>
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="text-sm font-semibold">{group.title}</div>
                  <div className="mt-1 text-xs text-muted">{t.common.modelsSharingSameQuota}</div>
                </div>
                <span className={cn('text-xs font-semibold', quotaTextColor(group.percent))}>{group.percent.toFixed(0)}%</span>
              </div>
              <div className="mt-2 text-base font-semibold">{group.remaining} / {group.total}</div>
              <Progress value={group.percent} className={cn('mt-3 h-2', quotaProgressColor(group.percent))} />
              <div className="mt-3 flex flex-wrap gap-2">
                {group.models.map((model) => (
                  <Badge key={model.id} variant="secondary" className="normal-case tracking-normal">
                    {model.displayName}
                  </Badge>
                ))}
              </div>
            </div>
          ))}
        </div>
      ) : !catalogError && !catalogLoading ? (
        <p className="text-sm text-muted">{t.dashboard.noData}</p>
      ) : null}

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

function QuotaAlert({ title, message }: { title: string; message: string }) {
  return (
    <Alert variant="warning">
      <AlertCircle className="h-4 w-4" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  )
}
