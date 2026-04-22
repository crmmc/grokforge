import type { Token } from '@/types'

export interface TokenQuotaMetric {
  key: string
  label: string
  shortLabel: string
  remaining: number
  total: number
  percent: number
}

/**
 * Build quota metrics from token's dynamic mode-based quotas.
 * Falls back to mode key as label/shortLabel when no translation provided.
 */
export function buildTokenQuotaMetrics(
  token: Token,
  modeLabels?: Record<string, string>,
): TokenQuotaMetric[] {
  const quotas = token.quotas ?? {}
  const limits = token.limit_quotas ?? {}
  const modes = Array.from(new Set([...Object.keys(quotas), ...Object.keys(limits)])).sort()

  return modes.map((mode) => {
    const remaining = quotas[mode] ?? 0
    const total = limits[mode] ?? remaining
    const label = modeLabels?.[mode] ?? mode
    const shortLabel = mode.charAt(0).toUpperCase()
    return createQuotaMetric(mode, label, shortLabel, remaining, total)
  })
}

export function quotaTextColor(percent: number): string {
  if (percent > 50) return 'text-emerald-700'
  if (percent > 20) return 'text-amber-700'
  return 'text-rose-700'
}

export function quotaSurfaceColor(percent: number): string {
  if (percent > 50) return 'border-emerald-200 bg-emerald-50/70'
  if (percent > 20) return 'border-amber-200 bg-amber-50/70'
  return 'border-rose-200 bg-rose-50/70'
}

export function quotaProgressColor(percent: number): string {
  if (percent > 50) return '[&>div]:bg-emerald-500 bg-emerald-100'
  if (percent > 20) return '[&>div]:bg-amber-500 bg-amber-100'
  return '[&>div]:bg-rose-500 bg-rose-100'
}

function createQuotaMetric(
  key: string,
  label: string,
  shortLabel: string,
  remaining: number,
  total: number,
): TokenQuotaMetric {
  const normalizedTotal = Math.max(total, remaining, 0)
  return {
    key,
    label,
    shortLabel,
    remaining,
    total: normalizedTotal,
    percent: quotaPercent(remaining, normalizedTotal),
  }
}

function quotaPercent(remaining: number, total: number): number {
  if (total <= 0) return 0
  return Math.max(0, Math.min(100, (remaining / total) * 100))
}
