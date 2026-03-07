'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'

interface SwitchProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'onChange' | 'size' | 'type'> {
  checked?: boolean
  onCheckedChange?: (checked: boolean) => void
}

const Switch = React.forwardRef<HTMLInputElement, SwitchProps>(
  ({ className, checked = false, onCheckedChange, disabled, ...props }, ref) => {
    return (
      <span
        className={cn(
          'relative inline-flex h-[22px] w-[44px] shrink-0',
          disabled && 'opacity-50',
          className
        )}
      >
        <input
          {...props}
          ref={ref}
          type="checkbox"
          role="switch"
          checked={checked}
          disabled={disabled}
          onChange={(e) => onCheckedChange?.(e.target.checked)}
          className={cn(
            'absolute inset-0 m-0 h-full w-full appearance-none rounded-full transition-all duration-150',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2',
            'disabled:cursor-not-allowed cursor-pointer',
            checked
              ? 'bg-primary shadow-[inset_0_1px_3px_rgba(0,0,0,0.15)]'
              : 'bg-transparent border-[1.5px] border-[rgba(0,0,0,0.35)]'
          )}
        />
        <span
          aria-hidden="true"
          className={cn(
            'pointer-events-none absolute top-1/2 rounded-full transition-all duration-150 -translate-y-1/2',
            checked
              ? 'left-[26px] h-[14px] w-[14px] bg-white'
              : 'left-[4px] h-[12px] w-[12px] bg-[rgba(0,0,0,0.45)]'
          )}
        />
      </span>
    )
  }
)
Switch.displayName = 'Switch'

export { Switch }
