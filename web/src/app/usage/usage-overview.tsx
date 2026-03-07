'use client'

import { useUsageStats } from '@/lib/hooks'
import {
  Card, CardContent, CardHeader, CardTitle,
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
  Skeleton, Progress,
} from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'
import { formatNumber } from '@/lib/utils'

type Period = 'hour' | 'day' | 'week' | 'month'

function StatCard({ title, value, subtitle, loading }: { title: string; value: string | number; subtitle?: string; loading?: boolean }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className="h-8 w-20" />
        ) : (
          <>
            <div className="text-2xl font-bold">{value}</div>
            {subtitle && <p className="text-xs text-muted">{subtitle}</p>}
          </>
        )}
      </CardContent>
    </Card>
  )
}

export function UsageOverview({ period }: { period: Period }) {
  const { data, isLoading } = useUsageStats(period)
  const { t } = useTranslation()

  const cacheTokens = data?.cache_tokens ?? 0
  const inputTokens = data?.tokens_input ?? 0
  const outputTokens = data?.tokens_output ?? 0
  const totalTokens = inputTokens + outputTokens + cacheTokens
  const totalRequests = data?.requests ?? 0
  const errorRate = totalRequests ? ((data?.errors ?? 0) / totalRequests * 100).toFixed(1) : '0'

  const periodLabels: Record<Period, string> = {
    hour: t.usage.lastHour,
    day: t.usage.today,
    week: t.usage.thisWeek,
    month: t.usage.thisMonth,
  }

  const byModelEntries = data?.by_model
    ? Object.entries(data.by_model).sort((a, b) => {
        const requestsDiff = b[1].requests - a[1].requests
        return requestsDiff !== 0 ? requestsDiff : a[0].localeCompare(b[0])
      })
    : []

  const byAPIKeyEntries = data?.by_api_key
    ? [...data.by_api_key].sort((a, b) => {
        const requestsDiff = b.requests - a.requests
        return requestsDiff !== 0 ? requestsDiff : a.api_key_name.localeCompare(b.api_key_name)
      })
    : []

  return (
    <div className="space-y-6">
      <div className="grid gap-4 grid-cols-2 md:grid-cols-3 lg:grid-cols-6">
        <StatCard title={t.usage.totalRequests} value={data?.requests ?? 0} loading={isLoading} />
        <StatCard title={t.usage.inputTokens} value={formatNumber(inputTokens)} loading={isLoading} />
        <StatCard title={t.usage.outputTokens} value={formatNumber(outputTokens)} loading={isLoading} />
        <StatCard title={t.usage.cacheTokens} value={formatNumber(cacheTokens)} loading={isLoading} />
        <StatCard title={t.usage.totalTokens} value={formatNumber(totalTokens)} loading={isLoading} />
        <StatCard
          title={t.usage.errorRate}
          value={`${errorRate}%`}
          subtitle={t.usage.errorsCount.replace('{count}', String(data?.errors ?? 0))}
          loading={isLoading}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t.usage.usageByModel.replace('{period}', periodLabels[period])}</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : byModelEntries.length === 0 ? (
            <p className="text-center text-muted py-8">
              {t.usage.noUsageData}
            </p>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t.usage.model}</TableHead>
                    <TableHead className="text-right">{t.usage.totalRequests}</TableHead>
                    <TableHead className="text-right">{t.usage.inputTokens}</TableHead>
                    <TableHead className="text-right">{t.usage.outputTokens}</TableHead>
                    <TableHead className="w-[200px]">{t.usage.share}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {byModelEntries.map(([model, stats]) => {
                    const share = totalRequests ? (stats.requests / totalRequests) * 100 : 0
                    return (
                      <TableRow key={model}>
                        <TableCell className="font-medium">{model}</TableCell>
                        <TableCell className="text-right">{stats.requests}</TableCell>
                        <TableCell className="text-right">{formatNumber(stats.tokens_input)}</TableCell>
                        <TableCell className="text-right">{formatNumber(stats.tokens_output)}</TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <Progress value={share} className="flex-1" />
                            <span className="text-sm text-muted w-12">{share.toFixed(0)}%</span>
                          </div>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t.usage.usageByAPIKey.replace('{period}', periodLabels[period])}</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : byAPIKeyEntries.length === 0 ? (
            <p className="text-center text-muted py-8">
              {t.usage.noUsageData}
            </p>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t.usage.apiKeyName}</TableHead>
                    <TableHead className="text-right">{t.usage.totalRequests}</TableHead>
                    <TableHead className="text-right">{t.usage.inputTokens}</TableHead>
                    <TableHead className="text-right">{t.usage.outputTokens}</TableHead>
                    <TableHead className="w-[200px]">{t.usage.share}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {byAPIKeyEntries.map((item) => {
                    const share = totalRequests ? (item.requests / totalRequests) * 100 : 0
                    return (
                      <TableRow key={item.api_key_name}>
                        <TableCell className="font-medium">{item.api_key_name}</TableCell>
                        <TableCell className="text-right">{item.requests}</TableCell>
                        <TableCell className="text-right">{formatNumber(item.tokens_input)}</TableCell>
                        <TableCell className="text-right">{formatNumber(item.tokens_output)}</TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <Progress value={share} className="flex-1" />
                            <span className="text-sm text-muted w-12">{share.toFixed(0)}%</span>
                          </div>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
