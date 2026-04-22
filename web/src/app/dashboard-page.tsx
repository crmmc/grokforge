'use client'

import dynamic from 'next/dynamic'
import { useSystemStatus, useAPIKeyStats, useDashboardTokenStats, useQuotaStats, useDashboardUsageStats } from '@/lib/hooks'
import { Card, CardContent, CardHeader, CardTitle, Skeleton, Progress, Alert, AlertDescription } from '@/components/ui'
import { AlertCircle } from 'lucide-react'
import { useTranslation } from '@/lib/i18n/context'
import { DashboardStatCards } from './dashboard-stat-cards'
import type { PoolQuota } from '@/types'

const UsageChart = dynamic(
  () => import('@/components/features/usage-chart').then((mod) => mod.UsageChart),
  {
    loading: () => (
      <Card>
        <CardContent className="pt-6">
          <Skeleton className="h-[300px] w-full" />
        </CardContent>
      </Card>
    ),
  }
)

function borderColorByRatio(current: number, total: number): string {
  if (total === 0) return 'border-l-zinc-400'
  const ratio = current / total
  if (ratio > 0.5) return 'border-l-emerald-500'
  if (ratio > 0.25) return 'border-l-amber-500'
  return 'border-l-rose-500'
}

function computeOverallDelta(delta: Record<string, number | null> | undefined): number | null {
  if (!delta) return null
  const vals = Object.values(delta).filter((v): v is number => v != null)
  if (vals.length === 0) return null
  return vals.reduce((a, b) => a + b, 0) / vals.length
}

function progressColor(remainPct: number): string {
  if (remainPct > 50) return '[&>div]:bg-emerald-500 bg-emerald-100'
  if (remainPct > 20) return '[&>div]:bg-amber-500 bg-amber-100'
  return '[&>div]:bg-rose-500 bg-rose-100'
}

function totalPoolQuota(pool: PoolQuota): number {
  if (!pool.mode_quotas) return 0
  return Object.values(pool.mode_quotas).reduce((sum, mq) => sum + mq.total_limit, 0)
}

function remainingPoolQuota(pool: PoolQuota): number {
  if (!pool.mode_quotas) return 0
  return Object.values(pool.mode_quotas).reduce((sum, mq) => sum + mq.total_remaining, 0)
}

function remainingPercent(pool: PoolQuota): number {
  const total = totalPoolQuota(pool)
  if (total <= 0) return 0
  return (remainingPoolQuota(pool) / total) * 100
}

export default function DashboardPage() {
  const { data: status, isLoading: statusLoading, error: statusError } = useSystemStatus()
  const { data: tokenStats, isLoading: tokensLoading, error: tokensError } = useDashboardTokenStats()
  const { data: apiKeyStats, isLoading: apiKeysLoading, error: apiKeysError } = useAPIKeyStats()
  const { data: quotaStats, isLoading: quotaLoading, error: quotaError } = useQuotaStats()
  const { data: usageStats, isLoading: usageLoading, error: usageError } = useDashboardUsageStats()
  const { t } = useTranslation()

  const overallDelta = computeOverallDelta(usageStats?.delta)
  const errors = [statusError, tokensError, apiKeysError, quotaError, usageError].filter(Boolean)

  return (
    <div className="space-y-8 max-w-6xl">
      <div className="flex flex-col gap-1">
        <h1 className="text-3xl font-bold tracking-tight">{t.dashboard.title}</h1>
        <p className="text-muted text-sm">{t.dashboard.description}</p>
      </div>

      {errors.length > 0 && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{t.common.loadFailed}{': '}{errors[0]?.message || t.common.unknownError}</AlertDescription>
        </Alert>
      )}

      {/* Top row: 5 stat cards */}
      <DashboardStatCards
        tokenStats={tokenStats}
        usageStats={usageStats}
        status={status}
        apiKeyStats={apiKeyStats}
        tokensLoading={tokensLoading}
        usageLoading={usageLoading}
        statusLoading={statusLoading}
        apiKeysLoading={apiKeysLoading}
        overallDelta={overallDelta}
        formatUptime={formatUptime}
        borderColorByRatio={borderColorByRatio}
      />

      {/* Middle row: quota + chart */}
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{t.dashboard.quota}</CardTitle>
          </CardHeader>
          <CardContent>
            {quotaLoading ? (
              <Skeleton className="h-24" />
            ) : !quotaStats?.pools?.length ? (
              <p className="text-sm text-muted">{t.dashboard.noData}</p>
            ) : (
              <div className="flex flex-col gap-4">
                {quotaStats.pools.map((pool) => {
                  const totalQuota = totalPoolQuota(pool)
                  const remainingQuota = remainingPoolQuota(pool)
                  const remainPct = remainingPercent(pool)

                  return (
                    <div key={pool.pool} className="rounded-lg border border-[rgba(0,0,0,0.06)] bg-[rgba(255,255,255,0.55)] p-4">
                      <div className="flex items-center justify-between text-sm">
                        <span className="font-medium">{poolLabel(pool.pool, t)}</span>
                        <span className="font-semibold">
                          {remainingQuota} / {totalQuota}
                        </span>
                      </div>
                      <div className="mt-1 flex items-center justify-between text-xs text-muted">
                        <span>{t.dashboard.quotaRemaining}</span>
                        <span>{remainPct.toFixed(0)}%</span>
                      </div>
                      <Progress value={remainPct} className={`mt-2 h-2 ${progressColor(remainPct)}`} />
                      <div className="mt-3 flex flex-wrap gap-3 text-xs text-muted sm:text-sm">
                        {Object.entries(pool.mode_quotas ?? {}).map(([mode, mq]) => (
                          <span key={mode}>{modeLabel(mode, t)}: {mq.total_remaining} / {mq.total_limit}</span>
                        ))}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </CardContent>
        </Card>

        <UsageChart
          title={t.dashboard.hourlyUsage}
          hourly={usageStats?.hourly}
          loading={usageLoading}
          labels={{ chat: t.dashboard.chat, image: t.dashboard.image, video: t.dashboard.video, noData: t.dashboard.noData }}
        />
      </div>

      {/* Bottom row: status breakdowns */}
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{t.dashboard.tokenStatus}</CardTitle>
          </CardHeader>
          <CardContent>
            {tokensLoading ? (
              <Skeleton className="h-24" />
            ) : (
              <div className="flex flex-col gap-3">
                <StatusRow label={t.tokens.active} value={tokenStats?.active ?? 0} dotClass="bg-emerald-500" />
                <StatusRow label={t.dashboard.disabled} value={tokenStats?.disabled ?? 0} dotClass="bg-zinc-400" />
                <StatusRow label={t.dashboard.exhausted} value={tokenStats?.exhausted ?? 0} dotClass="bg-amber-500" />
                <StatusRow label={t.dashboard.expired} value={tokenStats?.expired ?? 0} dotClass="bg-rose-500" />
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t.dashboard.apiKeyStatus}</CardTitle>
          </CardHeader>
          <CardContent>
            {apiKeysLoading ? (
              <Skeleton className="h-24" />
            ) : (
              <div className="flex flex-col gap-3">
                <StatusRow label={t.tokens.active} value={apiKeyStats?.active ?? 0} dotClass="bg-emerald-500" />
                <StatusRow label={t.dashboard.inactive} value={apiKeyStats?.inactive ?? 0} dotClass="bg-zinc-400" />
                <StatusRow label={t.dashboard.rateLimited} value={apiKeyStats?.rate_limited ?? 0} dotClass="bg-amber-500" />
                <StatusRow label={t.dashboard.expired} value={apiKeyStats?.expired ?? 0} dotClass="bg-rose-500" />
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function poolLabel(pool: string, t: ReturnType<typeof useTranslation>['t']): string {
  if (pool.toLowerCase().includes('basic')) return t.dashboard.basicPool
  if (pool.toLowerCase().includes('super')) return t.dashboard.superPool
  return pool
}

function modeLabel(mode: string, t: ReturnType<typeof useTranslation>['t']): string {
  switch (mode) {
    case 'chat': return t.dashboard.chat
    case 'image': return t.dashboard.image
    case 'video': return t.dashboard.video
    default: return mode
  }
}

function StatusRow({ label, value, dotClass }: { label: string; value: number; dotClass: string }) {
  return (
    <div className="flex items-center justify-between group">
      <div className="flex items-center gap-3">
        <div className={`h-2 w-2 rounded-full ${dotClass}`} />
        <span className="text-sm font-medium text-muted group-hover:text-foreground transition-colors">{label}</span>
      </div>
      <span className="font-semibold text-sm">{value}</span>
    </div>
  )
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  if (days > 0) return `${days}d ${hours}h`
  const minutes = Math.floor((seconds % 3600) / 60)
  if (hours > 0) return `${hours}h ${minutes}m`
  return `${minutes}m`
}
