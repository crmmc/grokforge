'use client'

import { AlertCircle } from 'lucide-react'
import { Alert, AlertDescription, AlertTitle, Badge, Card, CardContent, CardHeader, CardTitle, Progress, Skeleton, Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'
import type { AdminModelsResponse } from '@/lib/hooks/use-admin-models'
import { buildPoolQuotaGroups, buildQuotaCatalogPresentation, formatQuotaPresentationIssues, quotaProgressColor, quotaSurfaceColor, quotaTextColor } from '@/lib/quota-presentation'
import { useQuotaPresentationWarnings } from '@/lib/use-quota-presentation-warnings'
import type { QuotaStatsResponse } from '@/types'

interface DashboardQuotaPanelProps {
  catalog?: AdminModelsResponse
  catalogError?: Error | null
  catalogLoading: boolean
  quotaStats?: QuotaStatsResponse
  quotaError?: Error | null
  quotaLoading: boolean
}

interface PoolQuotaSection {
  pool: QuotaStatsResponse['pools'][number]
  groups: ReturnType<typeof buildPoolQuotaGroups>['groups']
  issues: ReturnType<typeof buildPoolQuotaGroups>['issues']
}

export function DashboardQuotaPanel({
  catalog,
  catalogError,
  catalogLoading,
  quotaStats,
  quotaError,
  quotaLoading,
}: DashboardQuotaPanelProps) {
  const { t } = useTranslation()
  const presentation = catalog ? buildQuotaCatalogPresentation(catalog) : null
  const poolSections = presentation
    ? (quotaStats?.pools ?? [])
        .map((pool) => ({
          pool,
          ...buildPoolQuotaGroups(pool.pool, pool.mode_quotas ?? {}, presentation),
        }))
        .sort((left, right) => poolOrder(left.pool.pool) - poolOrder(right.pool.pool))
    : []
  const warningMessages = poolSections.flatMap(({ pool, issues }) =>
    formatQuotaPresentationIssues(issues, t).map((message) => `${poolLabel(pool.pool, t)}: ${message}`)
  )

  useQuotaPresentationWarnings('dashboard', warningMessages)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t.dashboard.modelQuota}</CardTitle>
      </CardHeader>
      <CardContent>
        <QuotaPanelBody
          catalogError={catalogError}
          catalogLoading={catalogLoading}
          poolSections={poolSections}
          quotaError={quotaError}
          quotaLoading={quotaLoading}
          t={t}
        />
      </CardContent>
    </Card>
  )
}

function QuotaPanelBody({
  catalogError,
  catalogLoading,
  poolSections,
  quotaError,
  quotaLoading,
  t,
}: {
  catalogError?: Error | null
  catalogLoading: boolean
  poolSections: PoolQuotaSection[]
  quotaError?: Error | null
  quotaLoading: boolean
  t: ReturnType<typeof useTranslation>['t']
}) {
  const defaultPool = poolSections[0]?.pool.pool ?? ''

  if (quotaLoading || catalogLoading) {
    return <Skeleton className="h-24" />
  }
  if (quotaError) {
    return <InlineAlert title={t.common.loadFailed} message={quotaError.message || t.common.unknownError} variant="destructive" />
  }
  if (catalogError) {
    return <InlineAlert title={t.common.humanReadableQuotaUnavailable} message={catalogError.message || t.common.unknownError} variant="warning" />
  }
  if (!poolSections.length) {
    return <p className="text-sm text-muted">{t.dashboard.noData}</p>
  }

  return (
    <Tabs key={defaultPool} defaultValue={defaultPool} className="space-y-4">
      <TabsList className="h-auto flex-wrap gap-2 border-b-0 pb-0">
        {poolSections.map(({ pool }) => (
          <TabsTrigger
            key={pool.pool}
            value={pool.pool}
            className="rounded-full border border-[rgba(0,0,0,0.08)] px-3 py-1.5"
          >
            {poolLabel(pool.pool, t)}
          </TabsTrigger>
        ))}
      </TabsList>

      {poolSections.map((section) => (
        <TabsContent key={section.pool.pool} value={section.pool.pool} className="mt-0">
          <PoolQuotaCard section={section} t={t} />
        </TabsContent>
      ))}
    </Tabs>
  )
}

function PoolQuotaCard({
  section,
  t,
}: {
  section: PoolQuotaSection
  t: ReturnType<typeof useTranslation>['t']
}) {
  const { pool, groups, issues } = section

  return (
    <div className="rounded-lg border border-[rgba(0,0,0,0.06)] bg-[rgba(255,255,255,0.55)] p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="text-sm font-medium">{poolLabel(pool.pool, t)}</div>
          <div className="mt-1 flex flex-wrap gap-2 text-xs text-muted">
            <span>{t.tokens.active}: {pool.active_count}</span>
            <span>{t.common.total}: {pool.token_count}</span>
          </div>
        </div>
        {issues.length > 0 && (
          <Badge variant="warning">{t.common.mappingIssue}</Badge>
        )}
      </div>

      {issues.length > 0 && (
        <div className="mt-3">
          <InlineAlert
            title={t.common.humanReadableQuotaUnavailable}
            message={formatQuotaPresentationIssues(issues, t).join(' / ')}
            variant="warning"
          />
        </div>
      )}

      {groups.length === 0 ? (
        <p className="mt-3 text-sm text-muted">{t.dashboard.noData}</p>
      ) : (
        <div className="mt-3 grid gap-3">
          {groups.map((group) => (
            <div key={group.key} className={`rounded-lg border p-3 ${quotaSurfaceColor(group.percent)}`}>
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="text-sm font-semibold">{group.title}</div>
                  <div className="mt-1 text-xs text-muted">{t.common.modelsSharingSameQuota}</div>
                </div>
                <div className="text-right">
                  <div className="text-sm font-semibold">
                    {group.remaining} / {group.total}
                  </div>
                  <div className={`text-xs font-semibold ${quotaTextColor(group.percent)}`}>{group.percent.toFixed(0)}%</div>
                </div>
              </div>
              <Progress value={group.percent} className={`mt-3 h-2 ${quotaProgressColor(group.percent)}`} />
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
      )}
    </div>
  )
}

function InlineAlert({
  title,
  message,
  variant,
}: {
  title: string
  message: string
  variant: 'destructive' | 'warning'
}) {
  return (
    <Alert variant={variant}>
      <AlertCircle className="h-4 w-4" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  )
}

function poolLabel(pool: string, t: ReturnType<typeof useTranslation>['t']): string {
  if (pool.toLowerCase().includes('basic')) return t.dashboard.basicPool
  if (pool.toLowerCase().includes('super')) return t.dashboard.superPool
  if (pool.toLowerCase().includes('heavy')) return t.dashboard.heavyPool
  return pool
}

function poolOrder(pool: string): number {
  if (pool.toLowerCase().includes('basic')) return 1
  if (pool.toLowerCase().includes('super')) return 2
  if (pool.toLowerCase().includes('heavy')) return 3
  return 99
}
