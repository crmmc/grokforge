import type { Token } from '@/types'

export type TokenQuotaKey = 'chat' | 'image' | 'video'

export interface TokenQuotaMetric {
  key: TokenQuotaKey
  label: string
  shortLabel: string
  remaining: number
  total: number
  percent: number
}

interface TokenQuotaLabels {
  chat: string
  image: string
  video: string
}

export function buildTokenQuotaMetrics(token: Token, labels: TokenQuotaLabels): TokenQuotaMetric[] {
  return [
    createQuotaMetric('chat', labels.chat, 'C', token.chat_quota, token.total_chat_quota),
    createQuotaMetric('image', labels.image, 'I', token.image_quota, token.total_image_quota),
    createQuotaMetric('video', labels.video, 'V', token.video_quota, token.total_video_quota),
  ]
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
  key: TokenQuotaKey,
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
