'use client'

import * as React from 'react'
import { Upload, X } from 'lucide-react'
import { useTranslation } from '@/lib/i18n/context'
import { useToast } from '@/components/ui'

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
        className="border-2 border-dashed rounded-lg p-4 text-center cursor-pointer hover:border-primary transition-colors"
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
            <button
              type="button"
              className="absolute -top-2 -right-2 bg-destructive text-destructive-foreground rounded-full p-1"
              onClick={(e) => { e.stopPropagation(); clear() }}
              aria-label={t.common.delete}
            >
              <X className="h-3 w-3" />
            </button>
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
