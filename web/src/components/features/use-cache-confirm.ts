import { useState, useCallback } from 'react'
import { useToast } from '@/components/ui'
import { useDeleteCacheFiles, useClearCache } from '@/lib/hooks/use-cache'
import { useTranslation } from '@/lib/i18n/context'
import { formatSize } from '@/lib/utils'
import type { CacheMediaType, CacheStatsResponse } from '@/types/cache'

interface ConfirmAction {
  kind: 'delete' | 'clear'
  names?: string[]
}

export function useCacheConfirm(
  type: CacheMediaType,
  stats: CacheStatsResponse | undefined,
  typeLabel: string,
  setSelected: (s: Set<string>) => void,
  setPage: (p: number) => void,
) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const deleteMut = useDeleteCacheFiles()
  const clearMut = useClearCache()
  const [confirmAction, setConfirmAction] = useState<ConfirmAction | null>(null)

  const confirmDelete = useCallback((names: string[]) => {
    setConfirmAction({ kind: 'delete', names })
  }, [])

  const confirmClear = useCallback(() => {
    setConfirmAction({ kind: 'clear' })
  }, [])

  const executeConfirm = useCallback(() => {
    if (!confirmAction) return
    if (confirmAction.kind === 'delete' && confirmAction.names) {
      deleteMut.mutate({ type, names: confirmAction.names }, {
        onSuccess: (res) => {
          toast({ title: t.common.success, description: t.cache.deleteSuccess.replace('{count}', String(res.success)) })
          setSelected(new Set())
        },
        onError: () => { toast({ title: t.common.error, description: t.cache.deleteFailed, variant: 'destructive' }) },
      })
    } else if (confirmAction.kind === 'clear') {
      clearMut.mutate({ type }, {
        onSuccess: (res) => {
          toast({
            title: t.common.success,
            description: t.cache.clearSuccess.replace('{count}', String(res.deleted)).replace('{size}', `${res.freed_mb.toFixed(2)} MB`),
          })
          setSelected(new Set())
          setPage(1)
        },
        onError: () => { toast({ title: t.common.error, description: t.cache.clearFailed, variant: 'destructive' }) },
      })
    }
    setConfirmAction(null)
  }, [confirmAction, type, deleteMut, clearMut, toast, t, setSelected, setPage])

  const confirmDialogInfo = (() => {
    if (!confirmAction) return { title: '', desc: '' }
    if (confirmAction.kind === 'delete') {
      const count = confirmAction.names?.length ?? 0
      return { title: t.cache.delete, desc: t.cache.confirmDelete.replace('{count}', String(count)) }
    }
    const typeStats = stats?.[type]
    return {
      title: t.cache.clearAll,
      desc: t.cache.confirmClear
        .replace('{type}', typeLabel)
        .replace('{count}', String(typeStats?.count ?? 0))
        .replace('{size}', formatSize(typeStats?.size_mb ?? 0)),
    }
  })()

  return { confirmAction, setConfirmAction, confirmDelete, confirmClear, executeConfirm, confirmDialogInfo }
}
