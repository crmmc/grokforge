'use client'

import { useState } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { Input, Button } from '@/components/ui'
import { cn } from '@/lib/utils'

interface SensitiveInputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  className?: string
}

export function SensitiveInput({ className, ...props }: SensitiveInputProps) {
  const [visible, setVisible] = useState(false)

  return (
    <div className="relative">
      <Input
        type={visible ? 'text' : 'password'}
        className={cn('pr-10', className)}
        {...props}
      />
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="absolute right-0 top-0 h-full px-3 hover:bg-transparent"
        onClick={() => setVisible(!visible)}
        tabIndex={-1}
      >
        {visible ? (
          <EyeOff className="h-4 w-4 text-muted" />
        ) : (
          <Eye className="h-4 w-4 text-muted" />
        )}
      </Button>
    </div>
  )
}
