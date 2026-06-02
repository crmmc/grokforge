'use client'

import * as React from 'react'
import { Upload, X } from 'lucide-react'
import { useTranslation } from '@/lib/i18n/context'
import { Button, useToast } from '@/components/ui'

interface RefImageUploadProps {
  image: string | null
  onImageChange: (base64: string | null) => void
  label?: string
  maxHeight?: string
}

export function RefImageUpload({ image, onImageChange, label, maxHeight = 'max-h-24' }: RefImageUploadProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const fileInputRef = React.useRef<HTMLInputElement>(null)

  const readImageFile = (file: File | undefined) => {
    if (!file) {
      return
    }

    if (!file.type.startsWith('image/')) {
      toast({ title: t.common.error, description: t.function.invalidImageFile, variant: 'destructive' })
      return
    }

    const reader = new FileReader()
    reader.onload = () => {
      const result = typeof reader.result === 'string' ? reader.result.split(',')[1] : null
      onImageChange(result || null)
    }
    reader.readAsDataURL(file)
  }

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    readImageFile(e.target.files?.[0])
  }

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    readImageFile(e.dataTransfer.files[0])
  }

  const clear = () => {
    onImageChange(null)
    if (fileInputRef.current) fileInputRef.current.value = ''
  }

  return (
    <>
      <div
        role="button"
        tabIndex={0}
        aria-label={label || t.function.referenceImage}
        className="rounded-[4px] border-2 border-dashed border-[rgba(0,0,0,0.12)] bg-[rgba(255,255,255,0.45)] p-4 text-center cursor-pointer transition-all hover:border-primary hover:bg-[rgba(255,255,255,0.7)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2"
        onDragOver={(e) => e.preventDefault()}
        onDrop={handleDrop}
        onClick={() => fileInputRef.current?.click()}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            fileInputRef.current?.click()
          }
        }}
      >
        {image ? (
          <div className="relative inline-block">
            <img
              src={`data:image/png;base64,${image}`}
              alt={label || t.function.referenceImage}
              className={`${maxHeight} rounded`}
            />
            <Button
              type="button"
              variant="destructive"
              size="icon"
              className="absolute -top-2 -right-2 h-6 w-6 rounded-full p-0 [&_svg]:size-3"
              onClick={(e) => { e.stopPropagation(); clear() }}
              aria-label={t.common.delete}
            >
              <X className="h-3 w-3" />
            </Button>
          </div>
        ) : (
          <div className="text-muted">
            <Upload className="h-6 w-6 mx-auto mb-1" />
            <p className="text-xs">{t.function.dropImage}</p>
          </div>
        )}
      </div>
      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={handleFileSelect}
      />
    </>
  )
}
