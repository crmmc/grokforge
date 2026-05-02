'use client'

import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
  buttonVariants,
} from '@/components/ui'
import { ChevronDown, Download, CheckSquare } from 'lucide-react'
import { useTranslation } from '@/lib/i18n/context'
import type { BatchOperation } from '@/lib/hooks'
import { cn } from '@/lib/utils'

interface TokenActionsBarProps {
  selectedIds: Set<number>
  batchPending: boolean
  refreshPending: boolean
  onBatchOperation: (operation: BatchOperation) => void
  onBatchRefresh: () => void
  onExport: () => void
  onShowImport: () => void
  onSelectByStatus: (status: string) => void
  onDeselectAll: () => void
}

export function TokenActionsBar({ selectedIds, batchPending, refreshPending, onBatchOperation, onBatchRefresh, onExport, onShowImport, onSelectByStatus, onDeselectAll }: TokenActionsBarProps) {
  const { t } = useTranslation()

  return (
    <div className="flex flex-wrap gap-2 md:justify-end">
      <DropdownMenu>
        <DropdownMenuTrigger
          className={cn(
            buttonVariants({ variant: 'outline' }),
            'w-full sm:w-auto'
          )}
        >
          <CheckSquare className="mr-2 h-4 w-4" />
          <span>{t.tokens.selectByStatus}</span>
          <ChevronDown className="ml-2 h-4 w-4 text-muted" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-52">
          <DropdownMenuItem onClick={() => onSelectByStatus('active')}>{t.tokens.selectAllActive}</DropdownMenuItem>
          <DropdownMenuItem onClick={() => onSelectByStatus('exhausted')}>{t.tokens.selectAllExhausted}</DropdownMenuItem>
          <DropdownMenuItem onClick={() => onSelectByStatus('expired')}>{t.tokens.selectAllExpired}</DropdownMenuItem>
          <DropdownMenuItem onClick={() => onSelectByStatus('disabled')}>{t.tokens.selectAllDisabled}</DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={onDeselectAll}>{t.tokens.deselectAll}</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {selectedIds.size > 0 && (
        <DropdownMenu>
          <DropdownMenuTrigger
            disabled={batchPending || refreshPending}
            className={cn(
              buttonVariants({ variant: 'outline' }),
              'w-full min-w-[156px] justify-between sm:w-auto'
            )}
          >
            <span>{t.tokens.batch} ({selectedIds.size})</span>
            <ChevronDown className="h-4 w-4 text-muted" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            <DropdownMenuItem onClick={() => onBatchOperation('enable')}>{t.tokens.enable}</DropdownMenuItem>
            <DropdownMenuItem onClick={() => onBatchOperation('disable')}>{t.tokens.disable}</DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => onBatchOperation('enable_nsfw')}>{t.tokens.enableNsfw}</DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={onBatchRefresh} disabled={refreshPending}>
              {t.tokens.refreshQuota}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={() => onBatchOperation('delete')} variant="destructive">
              {t.common.delete}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      )}

      <Button variant="outline" onClick={onExport} className="w-full sm:w-auto">
        <Download className="mr-2 h-4 w-4" />
        {selectedIds.size > 0
          ? t.tokens.exportCount.replace('{count}', String(selectedIds.size))
          : t.tokens.exportAll}
      </Button>
      <Button onClick={onShowImport} className="w-full sm:w-auto">{t.tokens.importTokens}</Button>
    </div>
  )
}
