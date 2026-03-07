'use client'

import dynamic from 'next/dynamic'
import { useState, Suspense } from 'react'
import { useSearchParams } from 'next/navigation'
import { useTokens, useDeleteToken, useUpdateToken, useBatchTokens, useRefreshToken, useTokenIdsByStatus, type BatchOperation } from '@/lib/hooks'
import { Button, Skeleton, Alert, AlertDescription, ConfirmProvider, useConfirm } from '@/components/ui'
import { useToast } from '@/components/ui/toaster'
import { AlertCircle } from 'lucide-react'
import type { Token } from '@/types'
import { TokenActionsBar } from './token-actions-bar'
import { TokenTable } from '@/components/features/token-table'
import { useTranslation } from '@/lib/i18n/context'

const TokenDialog = dynamic(
  () => import('./token-dialog').then((mod) => mod.TokenDialog),
  { loading: () => null }
)

const ImportDialog = dynamic(
  () => import('./import-dialog').then((mod) => mod.ImportDialog),
  { loading: () => null }
)

function TokensPageInner() {
  const searchParams = useSearchParams()
  const statusFilter = searchParams.get('status') || undefined
  const nsfwFilter = searchParams.get('nsfw')
  const nsfwBool = nsfwFilter === 'true' ? true : nsfwFilter === 'false' ? false : undefined

  const [page, setPage] = useState(1)
  const [activeTokenID, setActiveTokenID] = useState<number | null>(null)
  const [showImport, setShowImport] = useState(false)
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const { data, isLoading, error } = useTokens({ page, page_size: 20, status: statusFilter, nsfw: nsfwBool })
  const deleteToken = useDeleteToken()
  const updateToken = useUpdateToken()
  const batchTokens = useBatchTokens()
  const refreshToken = useRefreshToken()
  const tokenIdsByStatus = useTokenIdsByStatus()
  const { toast } = useToast()
  const { t } = useTranslation()
  const confirm = useConfirm()

  const handleDelete = async (token: Token) => {
    if (!(await confirm({ title: `${t.common.delete} #${token.id}?`, variant: 'destructive' }))) return
    try {
      await deleteToken.mutateAsync(token.id)
      toast({ title: t.common.success, description: t.tokens.deleteSuccess.replace('{id}', String(token.id)) })
    } catch {
      toast({ title: t.common.error, description: t.common.error, variant: 'destructive' })
    }
  }

  const handleRefresh = async (token: Token) => {
    try {
      await refreshToken.mutateAsync(token.id)
      toast({ title: t.common.success, description: t.tokens.refreshed })
    } catch {
      toast({ title: t.common.error, description: t.tokens.refreshFailed, variant: 'destructive' })
    }
  }

  const handleToggleStatus = async (token: Token) => {
    const newStatus = token.status === 'active' ? 'disabled' : 'active'
    try {
      await updateToken.mutateAsync({ id: token.id, data: { status: newStatus } })
      toast({
        title: t.common.success,
        description: newStatus === 'active'
          ? t.tokens.enableSuccess.replace('{id}', String(token.id))
          : t.tokens.disableSuccess.replace('{id}', String(token.id)),
      })
    } catch {
      toast({ title: t.common.error, description: t.tokens.toggleFailed, variant: 'destructive' })
    }
  }

  const handleBatchOperation = async (operation: BatchOperation) => {
    const ids = Array.from(selectedIds)
    if (ids.length === 0) return

    const actionText = operation === 'delete' ? t.common.delete :
                       operation === 'enable' ? t.tokens.enable :
                       operation === 'disable' ? t.tokens.disable :
                       operation === 'enable_nsfw' ? t.tokens.enableNsfw :
                       operation === 'disable_nsfw' ? t.tokens.disableNsfw :
                       operation

    if (operation === 'delete' && !(await confirm({ title: `${t.common.delete} ${ids.length} tokens?`, variant: 'destructive' }))) {
      return
    }

    try {
      const result = await batchTokens.mutateAsync({ operation, ids })
      toast({
        title: t.common.success,
        description: t.tokens.batchResult
          .replace('{action}', actionText)
          .replace('{success}', String(result.success))
          .replace('{failed}', String(result.failed)),
      })
      setSelectedIds(new Set())
    } catch {
      toast({
        title: t.common.error,
        description: t.common.error,
        variant: 'destructive',
      })
    }
  }

  const handleExport = async () => {
    try {
      const ids = selectedIds.size > 0 ? Array.from(selectedIds) : undefined
      const result = await batchTokens.mutateAsync({ operation: 'export', ids, raw: true })
      const blob = new Blob([(result.raw_tokens || []).join('\n')], { type: 'text/plain' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `tokens-${new Date().toISOString().split('T')[0]}.txt`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      window.setTimeout(() => URL.revokeObjectURL(url), 0)
      toast({ title: t.common.success, description: t.tokens.exported.replace('{count}', String(result.success)) })
    } catch {
      toast({ title: t.common.error, description: t.tokens.exportFailed, variant: 'destructive' })
    }
  }

  const handleSelectByStatus = async (status: string) => {
    try {
      const result = await tokenIdsByStatus.mutateAsync(status)
      setSelectedIds(new Set(result.ids || []))
    } catch {
      toast({ title: t.common.error, description: t.common.operationFailed, variant: 'destructive' })
    }
  }

  return (
    <div className="space-y-8 max-w-6xl">
      <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div className="flex flex-col gap-1">
          <h1 className="text-3xl font-bold tracking-tight">{t.tokens.title}</h1>
          <p className="text-muted text-sm">{t.tokens.description}</p>
        </div>
        <TokenActionsBar
          selectedIds={selectedIds}
          batchPending={batchTokens.isPending}
          onBatchOperation={handleBatchOperation}
          onExport={handleExport}
          onShowImport={() => setShowImport(true)}
          onSelectByStatus={handleSelectByStatus}
          onDeselectAll={() => setSelectedIds(new Set())}
        />
      </div>

      {isLoading ? (
        <div className="rounded-md border border-[rgba(0,0,0,0.06)] shadow-sm bg-surface p-4 space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : error ? (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{t.common.loadFailed}{': '}{error.message || t.common.unknownError}</AlertDescription>
        </Alert>
      ) : (
        <TokenTable
          tokens={data?.data || []}
          selectedIds={selectedIds}
          onSelectionChange={setSelectedIds}
          onEdit={(token) => setActiveTokenID(token.id)}
          onDelete={handleDelete}
          onRefresh={handleRefresh}
          onToggleStatus={handleToggleStatus}
        />
      )}

      {data && data.total_pages > 1 && (
        <div className="flex justify-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page === 1}
            onClick={() => setPage((p) => p - 1)}
          >
            {t.common.previous}
          </Button>
          <span className="flex items-center px-2 text-sm">
            {t.common.pageInfo.replace('{page}', String(page)).replace('{total}', String(data.total_pages))}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={page === data.total_pages}
            onClick={() => setPage((p) => p + 1)}
          >
            {t.common.next}
          </Button>
        </div>
      )}

      {activeTokenID !== null && (
        <TokenDialog
          open={activeTokenID !== null}
          onOpenChange={(open) => !open && setActiveTokenID(null)}
          tokenId={activeTokenID}
        />
      )}

      {showImport && (
        <ImportDialog
          open={showImport}
          onOpenChange={setShowImport}
        />
      )}
    </div>
  )
}

function TokensPageContent() {
  return (
    <ConfirmProvider>
      <TokensPageInner />
    </ConfirmProvider>
  )
}

export default function TokensPage() {
  return (
    <Suspense fallback={<div className="space-y-8 max-w-6xl"><div className="animate-pulse bg-[rgba(0,0,0,0.04)] h-64 rounded" /></div>}>
      <TokensPageContent />
    </Suspense>
  )
}
