'use client'

import { HardDrive, Loader2, AlertCircle } from 'lucide-react'
import { Alert, AlertDescription, Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'
import { useCacheStats } from '@/lib/hooks'
import { CacheStatsCards } from '@/components/features/cache-stats-cards'
import { CacheFileTable } from '@/components/features/cache-file-table'

export default function CachePage() {
  const { t } = useTranslation()
  const { data: stats, isLoading, error } = useCacheStats()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <Loader2 className="h-8 w-8 animate-spin text-muted" />
      </div>
    )
  }

  if (error || !stats) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertDescription>
          {t.cache.errorLoading}{': '}
          {error?.message || t.common.unknownError}
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-4">
        <div className="rounded-lg bg-secondary p-2 mt-1 drop-shadow-sm">
          <HardDrive className="h-6 w-6 text-foreground" />
        </div>
        <div className="flex flex-col gap-1">
          <h1 className="text-3xl font-bold tracking-tight">{t.cache.title}</h1>
          <p className="text-muted text-sm">{t.cache.description}</p>
        </div>
      </div>

      <CacheStatsCards stats={stats} />

      <Tabs defaultValue="video" className="w-full">
        <TabsList className="mb-6">
          <TabsTrigger value="video">{t.cache.videoCache}</TabsTrigger>
          <TabsTrigger value="image">{t.cache.imageCache}</TabsTrigger>
        </TabsList>
        <TabsContent value="video">
          <CacheFileTable type="video" stats={stats} />
        </TabsContent>
        <TabsContent value="image">
          <CacheFileTable type="image" stats={stats} />
        </TabsContent>
      </Tabs>
    </div>
  )
}
