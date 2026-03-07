'use client'

import { VideoIcon } from 'lucide-react'
import { Card, CardContent } from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'
import type { CacheStatsResponse } from '@/types/cache'

function formatSize(mb: number): string {
  if (mb >= 1024) return `${(mb / 1024).toFixed(2)} GB`
  if (mb >= 1) return `${mb.toFixed(2)} MB`
  return `${(mb * 1024).toFixed(0)} KB`
}

interface CacheStatsCardsProps {
  stats: CacheStatsResponse
}

export function CacheStatsCards({ stats }: CacheStatsCardsProps) {
  const { t } = useTranslation()

  return (
    <div className="grid grid-cols-1 gap-4">
      <Card className="ring-2 ring-primary shadow-md">
        <CardContent className="flex items-center gap-4 p-6">
          <div className="rounded-lg p-3 bg-primary/8">
            <VideoIcon className="h-6 w-6 text-primary" />
          </div>
          <div>
            <p className="text-sm font-medium text-muted">{t.cache.videoCache}</p>
            <p className="text-2xl font-bold">{stats.video.count} <span className="text-sm font-normal text-muted">{t.cache.files}</span></p>
            <p className="text-sm text-muted">{formatSize(stats.video.size_mb)}</p>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
