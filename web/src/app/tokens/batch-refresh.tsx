'use client'

import { useState } from 'react'
import { useBatchRefresh, type BatchRefreshEvent } from '@/lib/hooks'
import { Button, Alert, AlertDescription } from '@/components/ui'
import { useToast } from '@/components/ui/toaster'
import { useConfirm } from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'

interface UseBatchRefreshFlowOptions {
  selectedIds: Set<number>
  onComplete: () => void
}

export function useBatchRefreshFlow({ selectedIds, onComplete }: UseBatchRefreshFlowOptions) {
  const { startRefresh, cancel: cancelRefresh, isRefreshing } = useBatchRefresh()
  const [refreshProgress, setRefreshProgress] = useState<BatchRefreshEvent | null>(null)
  const { toast } = useToast()
  const { t } = useTranslation()
  const confirm = useConfirm()

  const handleBatchRefresh = async () => {
    const ids = Array.from(selectedIds)
    if (ids.length === 0) return

    if (!(await confirm({ title: t.tokens.confirmBatchRefresh.replace('{count}', String(ids.length)) }))) return

    startRefresh(ids, {
      onStart: (total) => setRefreshProgress({ type: 'progress', current: 0, total }),
      onProgress: (evt) => setRefreshProgress(evt),
      onComplete: (evt) => {
        setRefreshProgress(null)
        toast({
          title: t.common.success,
          description: t.tokens.batchRefreshResult
            .replace('{success}', String(evt.success || 0))
            .replace('{failed}', String(evt.failed || 0)),
        })
        onComplete()
      },
      onCancel: () => {
        setRefreshProgress(null)
      },
      onError: (err) => {
        setRefreshProgress(null)
        toast({ title: t.common.error, description: err.message, variant: 'destructive' })
      },
    })
  }

  return { handleBatchRefresh, isRefreshing, refreshProgress, cancelRefresh }
}

export function RefreshProgressAlert({ progress, onCancel }: { progress: BatchRefreshEvent; onCancel: () => void }) {
  const { t } = useTranslation()

  return (
    <Alert>
      <AlertDescription className="flex items-center justify-between">
        <span>
          {t.tokens.refreshingProgress
            .replace('{current}', String(progress.current))
            .replace('{total}', String(progress.total))}
        </span>
        <Button variant="ghost" size="sm" onClick={onCancel}>
          {t.common.cancel}
        </Button>
      </AlertDescription>
    </Alert>
  )
}
