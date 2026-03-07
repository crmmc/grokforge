'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'
import { useTranslation } from '@/lib/i18n/context'
import { X } from 'lucide-react'

export interface ToastProps extends React.HTMLAttributes<HTMLDivElement> {
  variant?: 'default' | 'destructive'
  onClose?: () => void
}

const Toast = React.forwardRef<HTMLDivElement, ToastProps>(
  ({ className, variant = 'default', onClose, children, ...props }, ref) => {
    const { t } = useTranslation()

    return (
      <div
        ref={ref}
        role={variant === 'destructive' ? 'alert' : 'status'}
        aria-live={variant === 'destructive' ? 'assertive' : 'polite'}
        className={cn(
          'group pointer-events-auto relative flex w-full items-center justify-between space-x-2 overflow-hidden rounded-[8px] p-4 pr-6 shadow-[0_8px_32px_rgba(0,0,0,0.12)] backdrop-blur-[20px] saturate-120 border transition-all',
          variant === 'default' && 'bg-[rgba(255,255,255,0.9)] border-[rgba(0,0,0,0.08)] text-foreground',
          variant === 'destructive' && 'bg-destructive/95 border-destructive text-white',
          className
        )}
        {...props}
      >
        <div className="flex-1">{children}</div>
        {onClose && (
          <button
            type="button"
            onClick={onClose}
            className="absolute right-1.5 top-1.5 w-6 h-6 rounded-[4px] flex items-center justify-center opacity-0 transition-opacity hover:bg-[rgba(0,0,0,0.05)] group-hover:opacity-100"
            aria-label={t.common.close}
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
    )
  }
)
Toast.displayName = 'Toast'

function ToastTitle({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('text-[13px] font-semibold [&+div]:text-[12px]', className)} {...props} />
}

function ToastDescription({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('text-[13px] opacity-90', className)} {...props} />
}

export { Toast, ToastTitle, ToastDescription }
