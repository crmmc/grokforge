import { StatCard } from '@/components/features/stat-card'
import { Key, KeyRound, Activity, Zap, Coins } from 'lucide-react'
import { useTranslation } from '@/lib/i18n/context'
import { formatNumber } from '@/lib/utils'

interface DashboardStatCardsProps {
  tokenStats?: { active: number; total: number; disabled: number; cooling: number; expired: number }
  usageStats?: { total: number; today?: Record<string, number>; tokens_today?: { total: number; input: number; output: number; cache: number }; delta?: Record<string, number | null> }
  status?: { uptime: number; version?: string }
  apiKeyStats?: { active: number; total: number; inactive: number; rate_limited: number; expired: number }
  tokensLoading: boolean
  usageLoading: boolean
  statusLoading: boolean
  apiKeysLoading: boolean
  overallDelta: number | null
  formatUptime: (seconds: number) => string
  borderColorByRatio: (current: number, total: number) => string
}

export function DashboardStatCards({
  tokenStats, usageStats, status, apiKeyStats,
  tokensLoading, usageLoading, statusLoading, apiKeysLoading,
  overallDelta, formatUptime, borderColorByRatio,
}: DashboardStatCardsProps) {
  const { t } = useTranslation()
  const usageEntries = usageStats?.today
    ? Object.entries(usageStats.today)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([key, value]) => `${usageLabel(key, t)}: ${value}`)
        .join(', ')
    : undefined

  const tokenTotalsAvailable = usageStats?.tokens_today != null

  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-5">
      <StatCard
        title={t.dashboard.activeTokens}
        value={tokenStats?.active ?? 0}
        subtitle={`${tokenStats?.total ?? 0} ${t.dashboard.total}`}
        icon={Key}
        loading={tokensLoading}
        borderColor={borderColorByRatio(tokenStats?.active ?? 0, tokenStats?.total ?? 1)}
      />
      <StatCard
        title={t.dashboard.todayUsage}
        value={usageStats?.total ?? 0}
        subtitle={usageEntries}
        icon={Activity}
        loading={usageLoading}
        borderColor="border-l-blue-500"
        trend={overallDelta}
      />
      <StatCard
        title={t.dashboard.uptime}
        value={status ? formatUptime(status.uptime) : '-'}
        subtitle={status?.version ? `v${status.version}` : undefined}
        icon={Zap}
        loading={statusLoading}
        borderColor="border-l-violet-500"
      />
      <StatCard
        title={t.dashboard.activeKeys}
        value={apiKeyStats?.active ?? 0}
        subtitle={`${apiKeyStats?.total ?? 0} ${t.dashboard.total}`}
        icon={KeyRound}
        loading={apiKeysLoading}
        borderColor={borderColorByRatio(apiKeyStats?.active ?? 0, apiKeyStats?.total ?? 1)}
      />
      <StatCard
        title={t.dashboard.totalTokensToday}
        value={tokenTotalsAvailable ? formatNumber(usageStats?.tokens_today?.total ?? 0) : '-'}
        subtitle={
          tokenTotalsAvailable ? (
            <div className="flex flex-wrap items-center gap-2">
              <span className="inline-flex items-center rounded-sm bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-600 dark:text-emerald-400">
                {t.usage.inputTokens}: {formatNumber(usageStats?.tokens_today?.input ?? 0)}
              </span>
              <span className="inline-flex items-center rounded-sm bg-blue-500/10 px-1.5 py-0.5 text-[10px] font-medium text-blue-600 dark:text-blue-400">
                {t.usage.outputTokens}: {formatNumber(usageStats?.tokens_today?.output ?? 0)}
              </span>
              <span className="inline-flex items-center rounded-sm bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-medium text-amber-600 dark:text-amber-400">
                {t.usage.cacheTokens}: {formatNumber(usageStats?.tokens_today?.cache ?? 0)}
              </span>
            </div>
          ) : t.dashboard.noData
        }
        icon={Coins}
        loading={usageLoading}
        borderColor="border-l-cyan-500"
      />
    </div>
  )
}

function usageLabel(key: string, t: ReturnType<typeof useTranslation>['t']): string {
  switch (key) {
    case 'chat':
      return t.dashboard.chat
    case 'image':
      return t.dashboard.image
    case 'video':
      return t.dashboard.video
    default:
      return key
  }
}
