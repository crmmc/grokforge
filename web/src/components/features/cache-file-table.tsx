'use client'

import { useState, useCallback, useMemo } from 'react'
import { Eye, Download, Trash2, ChevronLeft, ChevronRight, PackageOpen } from 'lucide-react'
import {
  Button, Card, CardContent, Checkbox,
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
  Dialog, DialogContent, DialogHeader, DialogFooter, DialogTitle, DialogDescription,
} from '@/components/ui'
import { useToast } from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'
import { useCacheFiles, fetchCacheFileBlob } from '@/lib/hooks/use-cache'
import { CachePreviewDialog } from './cache-preview-dialog'
import { useCacheConfirm } from './use-cache-confirm'
import { formatBytes, formatDate } from '@/lib/utils'
import type { CacheFile, CacheMediaType, CacheStatsResponse } from '@/types/cache'

interface CacheFileTableProps {
  type: CacheMediaType
  stats: CacheStatsResponse | undefined
}

export function CacheFileTable({ type, stats }: CacheFileTableProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const [page, setPage] = useState(1)
  const pageSize = 50
  const { data, isLoading } = useCacheFiles(type, page, pageSize)

  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [previewFile, setPreviewFile] = useState<CacheFile | null>(null)

  const items = useMemo(() => data?.items ?? [], [data?.items])
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const typeLabel = t.cache.videoType

  const { confirmAction, setConfirmAction, confirmDelete, confirmClear, executeConfirm, confirmDialogInfo } =
    useCacheConfirm(type, stats, typeLabel, setSelected, setPage)

  const toggleSelect = useCallback((name: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }, [])

  const toggleAll = useCallback(() => {
    if (selected.size === items.length) setSelected(new Set())
    else setSelected(new Set(items.map((f) => f.name)))
  }, [items, selected.size])

  const handleDownload = useCallback(async (file: CacheFile) => {
    try {
      const blobUrl = await fetchCacheFileBlob(type, file.name, true)
      const a = document.createElement('a')
      a.href = blobUrl
      a.download = file.name
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      window.setTimeout(() => URL.revokeObjectURL(blobUrl), 0)
    } catch {
      toast({ title: t.common.error, description: t.cache.downloadFailed, variant: 'destructive' })
    }
  }, [type, toast, t])

  const handleBatchDownload = useCallback(async () => {
    const files = items.filter((f) => selected.has(f.name))
    for (const file of files) await handleDownload(file)
  }, [items, selected, handleDownload])

  return (
    <>
      <Card>
        <CardContent className="p-0">
          {/* Toolbar */}
          <div className="flex items-center justify-between px-4 py-3 border-b">
            <div className="flex w-full flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <Button variant="destructive" size="sm" onClick={confirmClear} disabled={total === 0} className="w-full sm:w-auto">{t.cache.clearAll}</Button>
            {selected.size > 0 && (
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm text-muted">{t.cache.selectedCount.replace('{count}', String(selected.size))}</span>
                <Button variant="outline" size="sm" onClick={handleBatchDownload}>
                  <Download className="h-4 w-4 mr-1" />{t.cache.batchDownload}
                </Button>
                <Button variant="destructive" size="sm" onClick={() => confirmDelete(Array.from(selected))}>
                  <Trash2 className="h-4 w-4 mr-1" />{t.cache.batchDelete}
                </Button>
              </div>
            )}
            </div>
          </div>
          {/* Table */}
          {isLoading ? (
            <div className="flex items-center justify-center h-48 text-muted">{t.common.loading}</div>
          ) : items.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-48 text-muted gap-3">
              <PackageOpen className="h-12 w-12" />
              <p>{t.cache.noFiles.replace('{type}', typeLabel)}</p>
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">
                    <Checkbox checked={selected.size === items.length && items.length > 0} onCheckedChange={toggleAll} />
                  </TableHead>
                  <TableHead>{t.cache.fileName}</TableHead>
                  <TableHead className="w-28">{t.cache.size}</TableHead>
                  <TableHead className="w-44">{t.cache.date}</TableHead>
                  <TableHead className="w-32 text-right">{t.cache.actions}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((file) => (
                  <TableRow key={file.name}>
                    <TableCell><Checkbox checked={selected.has(file.name)} onCheckedChange={() => toggleSelect(file.name)} /></TableCell>
                    <TableCell className="font-mono text-sm truncate max-w-xs">{file.name}</TableCell>
                    <TableCell className="text-sm">{formatBytes(file.size_bytes)}</TableCell>
                    <TableCell className="text-sm">{formatDate(file.mod_time_ms)}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex items-center justify-end gap-1">
                        <Button variant="ghost" size="sm" onClick={() => setPreviewFile(file)} title={t.cache.view} aria-label={t.cache.view}><Eye className="h-4 w-4" /></Button>
                        <Button variant="ghost" size="sm" onClick={() => handleDownload(file)} title={t.cache.download} aria-label={t.cache.download}><Download className="h-4 w-4" /></Button>
                        <Button variant="ghost" size="sm" onClick={() => confirmDelete([file.name])} title={t.cache.delete} aria-label={t.cache.delete} className="text-destructive hover:text-destructive"><Trash2 className="h-4 w-4" /></Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-center gap-2 py-3 border-t">
              <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
                <span className="sr-only">{t.common.previous}</span>
                <ChevronLeft className="h-4 w-4" />
              </Button>
              <span className="text-sm text-muted">{page} / {totalPages}</span>
              <Button variant="outline" size="sm" disabled={page >= totalPages} onClick={() => setPage((p) => Math.min(totalPages, p + 1))}>
                <span className="sr-only">{t.common.next}</span>
                <ChevronRight className="h-4 w-4" />
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
      <CachePreviewDialog open={!!previewFile} onClose={() => setPreviewFile(null)} file={previewFile} type={type} />
      <Dialog open={!!confirmAction} onOpenChange={(v) => !v && setConfirmAction(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{confirmDialogInfo.title}</DialogTitle>
            <DialogDescription>{confirmDialogInfo.desc}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmAction(null)}>{t.common.cancel}</Button>
            <Button variant="destructive" onClick={executeConfirm}>{t.common.confirm}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
