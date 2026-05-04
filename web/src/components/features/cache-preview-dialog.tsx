'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'
import { fetchCacheFileBlob } from '@/lib/hooks/use-cache'
import type { CacheFile, CacheMediaType } from '@/types/cache'
import { formatBytes } from '@/lib/utils'
import { Loader2 } from 'lucide-react'

interface CachePreviewDialogProps {
  open: boolean
  onClose: () => void
  file: CacheFile | null
  type: CacheMediaType
}

export function CachePreviewDialog({ open, onClose, file, type }: CachePreviewDialogProps) {
  const { t } = useTranslation()
  const [blobUrl, setBlobUrl] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [loadError, setLoadError] = useState(false)
  const blobUrlRef = useRef<string | null>(null)

  const clearBlobUrl = useCallback(() => {
    if (blobUrlRef.current) {
      URL.revokeObjectURL(blobUrlRef.current)
      blobUrlRef.current = null
    }
    setBlobUrl(null)
  }, [])

  useEffect(() => {
    if (!open || !file) {
      clearBlobUrl()
      setLoading(false)
      setLoadError(false)
      return
    }

    let active = true
    clearBlobUrl()
    setLoading(true)
    setLoadError(false)
    fetchCacheFileBlob(type, file.name)
      .then((url) => {
        if (!active) {
          URL.revokeObjectURL(url)
        } else {
          blobUrlRef.current = url
          setBlobUrl(url)
        }
      })
      .catch(() => {
        if (active) setLoadError(true)
      })
      .finally(() => {
        if (active) setLoading(false)
      })

    return () => {
      active = false
      clearBlobUrl()
    }
  }, [open, file, type, clearBlobUrl])

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t.cache.preview}</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col items-center gap-3">
          {loading && (
            <div className="flex items-center justify-center h-48">
              <Loader2 className="h-8 w-8 animate-spin text-muted" />
            </div>
          )}
          {!loading && loadError && (
            <div className="rounded-md border border-destructive/30 bg-destructive/5 px-4 py-6 text-center text-sm text-destructive">
              {t.cache.previewFailed}
            </div>
          )}
          {!loading && blobUrl && type === 'video' && (
            <video src={blobUrl} controls className="max-w-full max-h-[70vh] rounded-lg" />
          )}
          {!loading && blobUrl && type === 'image' && (
            <img src={blobUrl} alt={file?.name ?? 'Preview'} className="max-w-full max-h-[70vh] rounded-lg" />
          )}
          {file && (
            <div className="text-sm text-muted text-center">
              <span className="font-medium">{file.name}</span>
              <span className="mx-2">-</span>
              <span>{formatBytes(file.size_bytes)}</span>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
