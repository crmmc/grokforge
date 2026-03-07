'use client'

import { useState, useRef, useCallback } from 'react'
import { X } from 'lucide-react'
import { Input } from './input'
import { useTranslation } from '@/lib/i18n/context'

const HTTP_CODE_RE = /^\d{3}$/

interface StatusCodeTagInputProps {
  id?: string
  codes: number[]
  onChange: (codes: number[]) => void
  placeholder?: string
  errorMsg?: string
}

export function StatusCodeTagInput({ id, codes, onChange, placeholder, errorMsg }: StatusCodeTagInputProps) {
  const { t } = useTranslation()
  const [inputValue, setInputValue] = useState('')
  const [error, setError] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  const addCode = useCallback((raw: string) => {
    const value = raw.trim()
    if (!value) return
    if (!HTTP_CODE_RE.test(value)) {
      setError(errorMsg ?? t.config.statusCodeFormatError)
      return
    }
    const num = Number(value)
    if (num < 100 || num > 599) {
      setError(errorMsg ?? t.config.statusCodeRangeError)
      return
    }
    if (codes.includes(num)) {
      setError(t.config.valueAlreadyExists.replace('{value}', value))
      return
    }
    setError('')
    onChange([...codes, num])
    setInputValue('')
  }, [codes, onChange, errorMsg, t])

  const removeCode = useCallback((index: number) => {
    onChange(codes.filter((_, i) => i !== index))
  }, [codes, onChange])

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      addCode(inputValue)
    }
    if (e.key === 'Backspace' && !inputValue && codes.length > 0) {
      removeCode(codes.length - 1)
    }
  }

  const handlePaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData('text')
    if (text.includes('\n') || text.includes(',') || text.includes(' ')) {
      e.preventDefault()
      const parts = text.split(/[\n,\s]+/).map(s => s.trim()).filter(Boolean)
      const newCodes = [...codes]
      for (const part of parts) {
        if (HTTP_CODE_RE.test(part)) {
          const num = Number(part)
          if (num >= 100 && num <= 599 && !newCodes.includes(num)) {
            newCodes.push(num)
          }
        }
      }
      onChange(newCodes)
      setInputValue('')
      setError('')
    }
  }

  return (
    <div>
      <div
        className="min-h-[42px] rounded-md border border-input bg-background px-2 py-1.5 cursor-text flex flex-wrap items-center gap-1.5"
        onClick={() => inputRef.current?.focus()}
      >
        {codes.map((code, i) => (
          <span
            key={`${code}-${i}`}
            className="inline-flex items-center gap-1 rounded-md bg-secondary px-2 py-0.5 text-xs font-mono text-secondary-foreground"
          >
            {code}
            <button
              type="button"
              onClick={(e) => { e.stopPropagation(); removeCode(i) }}
              className="ml-0.5 rounded-sm hover:bg-destructive/20 hover:text-destructive transition-colors"
              aria-label={t.common.delete}
            >
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}
        <Input
          ref={inputRef}
          id={id}
          value={inputValue}
          onChange={(e) => { setInputValue(e.target.value); setError('') }}
          onKeyDown={handleKeyDown}
          onPaste={handlePaste}
          placeholder={codes.length === 0 ? (placeholder ?? '429') : ''}
          className="border-0 shadow-none h-7 w-20 min-w-[60px] flex-1 px-0 text-sm font-mono focus-visible:ring-0"
          maxLength={3}
        />
      </div>
      {error && <p className="text-sm text-destructive mt-1">{error}</p>}
    </div>
  )
}
