'use client'

import { useState, useRef, useCallback } from 'react'
import { X } from 'lucide-react'
import { Input } from './input'
import { useTranslation } from '@/lib/i18n/context'

const MODEL_RE = /^[a-zA-Z0-9\-_:.#]+$/

interface ModelTagInputProps {
  id?: string
  models: string[]
  onChange: (models: string[]) => void
  placeholder?: string
}

export function ModelTagInput({ id, models, onChange, placeholder }: ModelTagInputProps) {
  const { t } = useTranslation()
  const [inputValue, setInputValue] = useState('')
  const [error, setError] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const addModel = useCallback((raw: string) => {
    const value = raw.trim()
    if (!value) return
    if (!MODEL_RE.test(value)) {
      setError(t.config.invalidModelCharacters.replace('{value}', value))
      return
    }
    if (models.includes(value)) {
      setError(t.config.valueAlreadyExists.replace('{value}', value))
      return
    }
    setError('')
    onChange([...models, value])
    setInputValue('')
  }, [models, onChange, t])

  const removeModel = useCallback((index: number) => {
    onChange(models.filter((_, i) => i !== index))
  }, [models, onChange])

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      addModel(inputValue)
    }
  }

  const handlePaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData('text')
    if (text.includes('\n')) {
      e.preventDefault()
      const lines = text.split('\n').map(s => s.trim()).filter(Boolean)
      const newModels = [...models]
      for (const line of lines) {
        if (MODEL_RE.test(line) && !newModels.includes(line)) {
          newModels.push(line)
        }
      }
      onChange(newModels)
      setInputValue('')
      setError('')
    }
  }

  return (
    <div>
      <div
        className="min-h-[120px] max-h-[240px] overflow-y-auto rounded-md border border-input bg-background p-2 cursor-text"
        onClick={() => inputRef.current?.focus()}
      >
        <div className="flex flex-wrap gap-1.5 mb-1.5">
          {models.map((model, i) => (
            <span
              key={`${model}-${i}`}
              className="inline-flex items-center gap-1 rounded-md bg-secondary px-2 py-0.5 text-xs font-mono text-secondary-foreground"
            >
              {model}
              <button
                type="button"
                onClick={(e) => { e.stopPropagation(); removeModel(i) }}
                className="ml-0.5 rounded-sm hover:bg-destructive/20 hover:text-destructive transition-colors"
                aria-label={t.common.delete}
              >
                <X className="h-3 w-3" />
              </button>
            </span>
          ))}
        </div>
        <Input
          ref={inputRef}
          id={id}
          value={inputValue}
          onChange={(e) => { setInputValue(e.target.value); setError('') }}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          placeholder={models.length === 0 ? placeholder : ''}
          className="border-0 shadow-none h-7 px-0 text-sm font-mono focus-visible:ring-0"
        />
      </div>
      {error && <p className="text-sm text-red-500 mt-1">{error}</p>}
      <p className="text-sm text-muted mt-1">
        {t.config.modelsCount.replace('{count}', String(models.length))}
      </p>
    </div>
  )
}
