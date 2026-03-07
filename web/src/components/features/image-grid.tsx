'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'
import { Button, useToast } from '@/components/ui'
import { Download, X, ChevronLeft, ChevronRight } from 'lucide-react'
import type { GeneratedImage } from '@/lib/function-api'
import { useTranslation } from '@/lib/i18n/context'

interface ImageGridProps {
  images: GeneratedImage[]
  className?: string
}

export function ImageGrid({ images, className }: ImageGridProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const [lightboxIndex, setLightboxIndex] = React.useState<number | null>(null)
  const lightboxRef = React.useRef<HTMLDivElement>(null)
  const closeButtonRef = React.useRef<HTMLButtonElement>(null)
  const lastFocusedElementRef = React.useRef<HTMLElement | null>(null)

  const getImageSrc = (image: GeneratedImage): string => {
    if (image.url) return image.url
    if (image.b64_json) return `data:image/png;base64,${image.b64_json}`
    return ''
  }

  const handleDownload = async (image: GeneratedImage, index: number) => {
    const src = getImageSrc(image)
    if (!src) return

    try {
      let blob: Blob
      if (src.startsWith('data:')) {
        const res = await fetch(src)
        blob = await res.blob()
      } else {
        const res = await fetch(src)
        blob = await res.blob()
      }

      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `grokforge-image-${Date.now()}-${index}.png`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      window.setTimeout(() => URL.revokeObjectURL(url), 0)
    } catch {
      toast({ title: t.common.error, description: t.function.downloadFailed, variant: 'destructive' })
    }
  }

  const openLightbox = (index: number) => {
    lastFocusedElementRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    setLightboxIndex(index)
  }

  const goToPrev = React.useCallback(() => {
    setLightboxIndex((prev) => {
      if (prev === null) return null
      return prev > 0 ? prev - 1 : images.length - 1
    })
  }, [images.length])

  const goToNext = React.useCallback(() => {
    setLightboxIndex((prev) => {
      if (prev === null) return null
      return prev < images.length - 1 ? prev + 1 : 0
    })
  }, [images.length])

  const closeLightbox = React.useCallback(() => setLightboxIndex(null), [])

  // Keyboard navigation
  React.useEffect(() => {
    if (lightboxIndex === null) return

    const previousOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    closeButtonRef.current?.focus()

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeLightbox()
      if (e.key === 'ArrowLeft') goToPrev()
      if (e.key === 'ArrowRight') goToNext()

      if (e.key === 'Tab') {
        const focusable = lightboxRef.current?.querySelectorAll<HTMLElement>(
          'button,[href],input,select,textarea,[tabindex]:not([tabindex="-1"])'
        )

        if (!focusable?.length) {
          e.preventDefault()
          return
        }

        const first = focusable[0]
        const last = focusable[focusable.length - 1]
        const active = document.activeElement

        if (e.shiftKey && active === first) {
          e.preventDefault()
          last.focus()
        } else if (!e.shiftKey && active === last) {
          e.preventDefault()
          first.focus()
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      window.removeEventListener('keydown', handleKeyDown)
      lastFocusedElementRef.current?.focus()
    }
  }, [lightboxIndex, goToPrev, goToNext, closeLightbox])

  if (images.length === 0) {
    return (
      <div className={cn('flex items-center justify-center h-64 text-muted', className)}>
        {t.function.noImagesGenerated}
      </div>
    )
  }

  return (
    <>
      <div className={cn('grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4', className)}>
        {images.map((image, index) => (
          <div key={index} className="group relative aspect-square rounded-lg overflow-hidden bg-[rgba(0,0,0,0.04)]">
            <button type="button" className="h-full w-full" onClick={() => openLightbox(index)} aria-label={`${t.common.expand} ${t.function.generatedImages} ${index + 1}`}>
              <img
                src={getImageSrc(image)}
                alt={`Generated image ${index + 1}`}
                className="w-full h-full object-cover cursor-pointer transition-transform hover:scale-105"
              />
            </button>
            <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
              <Button
                type="button"
                size="sm"
                variant="secondary"
                onClick={(e) => {
                  e.stopPropagation()
                  handleDownload(image, index)
                }}
              >
                <Download className="h-4 w-4 mr-1" />
                {t.function.download}
              </Button>
            </div>
          </div>
        ))}
      </div>

      {/* Lightbox */}
      {lightboxIndex !== null && (
        <div
          ref={lightboxRef}
          role="dialog"
          aria-modal="true"
          aria-label={t.function.generatedImages}
          tabIndex={-1}
          className="fixed inset-0 z-50 bg-black/90 flex items-center justify-center"
          onClick={closeLightbox}
        >
          <button
            ref={closeButtonRef}
            type="button"
            className="absolute top-4 right-4 text-white hover:text-gray-300 transition-colors"
            onClick={closeLightbox}
            aria-label={t.common.close}
          >
            <X className="h-8 w-8" />
          </button>

          {images.length > 1 && (
            <>
              <button
                type="button"
                className="absolute left-4 top-1/2 -translate-y-1/2 text-white hover:text-gray-300 transition-colors"
                onClick={(e) => { e.stopPropagation(); goToPrev() }}
                aria-label={t.common.previous}
              >
                <ChevronLeft className="h-10 w-10" />
              </button>
              <button
                type="button"
                className="absolute right-4 top-1/2 -translate-y-1/2 text-white hover:text-gray-300 transition-colors"
                onClick={(e) => { e.stopPropagation(); goToNext() }}
                aria-label={t.common.next}
              >
                <ChevronRight className="h-10 w-10" />
              </button>
            </>
          )}

          <img
            src={getImageSrc(images[lightboxIndex])}
            alt={`Generated image ${lightboxIndex + 1}`}
            className="max-w-[90vw] max-h-[90vh] object-contain"
            onClick={(e) => e.stopPropagation()}
          />

          <div className="absolute bottom-4 left-1/2 -translate-x-1/2 flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="secondary"
              onClick={(e) => {
                e.stopPropagation()
                handleDownload(images[lightboxIndex], lightboxIndex)
              }}
            >
              <Download className="h-4 w-4 mr-1" />
              {t.function.download}
            </Button>
            <span className="text-white text-sm self-center ml-2">
              {lightboxIndex + 1} / {images.length}
            </span>
          </div>
        </div>
      )}
    </>
  )
}
