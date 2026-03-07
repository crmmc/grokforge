'use client'

import dynamic from 'next/dynamic'
import { useState } from 'react'
import { useAPIKeys, useDeleteAPIKey, useRegenerateAPIKey } from '@/lib/hooks'
import {
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
  Button, Skeleton, Alert, AlertDescription, StatusBadge, ConfirmProvider, useConfirm,
  Tooltip, TooltipTrigger, TooltipContent,
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger, DropdownMenuSeparator
} from '@/components/ui'
import { useToast } from '@/components/ui/toaster'
import { Pencil, Trash2, Plus, Copy, RefreshCw, MoreHorizontal, AlertCircle } from 'lucide-react'
import type { APIKey } from '@/types'
import { useTranslation } from '@/lib/i18n/context'

const APIKeyDialog = dynamic(
  () => import('./apikey-dialog').then((mod) => mod.APIKeyDialog),
  { loading: () => null }
)

function formatDateTime(value: string | null): string {
  if (!value) return '-'
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value))
}

function APIKeysPageInner() {
  const [page, setPage] = useState(1)
  const [editKey, setEditKey] = useState<APIKey | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const { data, isLoading, error } = useAPIKeys({ page, page_size: 20 })
  const deleteKey = useDeleteAPIKey()
  const regenerateKey = useRegenerateAPIKey()
  const { toast } = useToast()
  const { t } = useTranslation()
  const confirm = useConfirm()

  const handleDelete = async (id: number, name: string) => {
    if (!(await confirm({ title: `${t.common.delete} "${name}"?`, variant: 'destructive' }))) return
    try {
      await deleteKey.mutateAsync(id)
      toast({ title: t.common.success, description: t.apiKeys.deleteSuccess.replace('{name}', name) })
    } catch {
      toast({ title: t.common.error, description: t.common.error, variant: 'destructive' })
    }
  }

  const copyKey = async (key: string) => {
    try {
      await navigator.clipboard.writeText(key)
      toast({ title: t.common.copied, description: t.apiKeys.copiedToClipboard })
    } catch {
      toast({ title: t.common.error, description: t.common.operationFailed, variant: 'destructive' })
    }
  }

  const maskKey = (key: string) => {
    if (key.length <= 8) return key
    return `${key.slice(0, 4)}...${key.slice(-4)}`
  }

  return (
    <div className="space-y-8 max-w-6xl">
      <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
        <div className="flex flex-col gap-1">
          <h1 className="text-3xl font-bold tracking-tight">{t.apiKeys.title}</h1>
          <p className="text-muted text-sm">{t.apiKeys.description}</p>
        </div>
        <Button onClick={() => setShowCreate(true)} className="w-full sm:w-auto">
          <Plus className="h-4 w-4 mr-1" /> {t.apiKeys.createKey}
        </Button>
      </div>

      {error ? (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{t.common.loadFailed}{': '}{error.message || t.common.unknownError}</AlertDescription>
        </Alert>
      ) : (
      <div className="rounded-md border border-[rgba(0,0,0,0.06)] shadow-sm bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="whitespace-nowrap min-w-[120px]">{t.apiKeys.name}</TableHead>
              <TableHead className="whitespace-nowrap min-w-[140px]">{t.apiKeys.key}</TableHead>
              <TableHead className="whitespace-nowrap min-w-[100px]">{t.apiKeys.status}</TableHead>
              <TableHead className="whitespace-nowrap min-w-[120px]">{t.apiKeys.dailyUsage}</TableHead>
              <TableHead className="whitespace-nowrap min-w-[100px]">{t.apiKeys.totalUsed}</TableHead>
              <TableHead className="whitespace-nowrap min-w-[140px]">{t.apiKeys.lastUsed}</TableHead>
              <TableHead className="w-[100px] text-right whitespace-nowrap">{t.apiKeys.actions}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 7 }).map((_, j) => (
                    <TableCell key={j}><Skeleton className="h-4 w-20" /></TableCell>
                  ))}
                </TableRow>
              ))
            ) : data?.data.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="text-center text-muted">
                  {t.apiKeys.noKeys}
                </TableCell>
              </TableRow>
            ) : (
              data?.data.map((apiKey) => (
                <TableRow key={apiKey.id}>
                  <TableCell className="font-medium whitespace-nowrap">{apiKey.name}</TableCell>
                  <TableCell className="whitespace-nowrap">
                    <div className="flex items-center gap-1">
                      <code className="text-xs">{maskKey(apiKey.key)}</code>
                      <Button variant="ghost" size="icon" className="h-6 w-6" onClick={() => void copyKey(apiKey.key)} aria-label={t.common.copy}>
                        <Copy className="h-3 w-3" />
                      </Button>
                    </div>
                  </TableCell>
                  <TableCell>
                    <StatusBadge
                      status={apiKey.status}
                      label={apiKey.status === 'active' ? t.apiKeys.active :
                        apiKey.status === 'expired' ? t.apiKeys.expired :
                          apiKey.status === 'rate_limited' ? t.apiKeys.rateLimited :
                            t.apiKeys.inactive}
                    />
                  </TableCell>
                  <TableCell className="whitespace-nowrap">
                    {apiKey.daily_used} / {apiKey.daily_limit || '∞'}
                  </TableCell>
                  <TableCell className="whitespace-nowrap">{apiKey.total_used}</TableCell>
                  <TableCell className="whitespace-nowrap">
                    {formatDateTime(apiKey.last_used_at)}
                  </TableCell>
                  <TableCell className="w-[100px] whitespace-nowrap">
                    <div className="flex items-center gap-1 justify-end min-w-max">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button variant="ghost" size="icon" onClick={() => setEditKey(apiKey)} aria-label={t.common.edit} className="h-8 w-8 text-muted hover:text-foreground">
                            <Pencil className="h-4 w-4" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>{t.common.edit}</TooltipContent>
                      </Tooltip>
                      <DropdownMenu>
                        <DropdownMenuTrigger className="inline-flex items-center justify-center h-8 w-8 rounded-md text-muted hover:text-foreground hover:bg-[rgba(0,0,0,0.03)] focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring" aria-label={t.apiKeys.actions}>
                          <MoreHorizontal className="h-4 w-4" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-40">
                          <DropdownMenuItem
                            onClick={async () => {
                              try {
                                await regenerateKey.mutateAsync(apiKey.id)
                                toast({ title: t.common.success, description: t.apiKeys.regenerated })
                              } catch {
                                toast({ title: t.common.error, description: t.apiKeys.regenerateFailed, variant: 'destructive' })
                              }
                            }}
                          >
                            <RefreshCw className="mr-2 h-4 w-4" /> {t.apiKeys.regenerate}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem onClick={() => handleDelete(apiKey.id, apiKey.name)} className="text-destructive focus:text-destructive">
                            <Trash2 className="mr-2 h-4 w-4" /> {t.common.delete}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      )}

      {data && data.total_pages > 1 && (
        <div className="flex justify-center gap-2">
          <Button variant="outline" size="sm" disabled={page === 1} onClick={() => setPage((p) => p - 1)}>
            {t.common.previous}
          </Button>
          <span className="flex items-center px-2 text-sm">{t.common.pageInfo.replace('{page}', String(page)).replace('{total}', String(data.total_pages))}</span>
          <Button variant="outline" size="sm" disabled={page === data.total_pages} onClick={() => setPage((p) => p + 1)}>
            {t.common.next}
          </Button>
        </div>
      )}

      {showCreate && <APIKeyDialog open={showCreate} onOpenChange={setShowCreate} mode="create" />}

      {editKey && (
        <APIKeyDialog
          open={!!editKey}
          onOpenChange={(open) => !open && setEditKey(null)}
          mode="edit"
          apiKey={editKey}
        />
      )}
    </div>
  )
}

export default function APIKeysPage() {
  return (
    <ConfirmProvider>
      <APIKeysPageInner />
    </ConfirmProvider>
  )
}
