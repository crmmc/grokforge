import * as React from 'react'
import { cn } from '@/lib/utils'

interface ProgressProps extends React.HTMLAttributes<HTMLDivElement> {
  value?: number
  indeterminate?: boolean
}

function Progress({ className, value, indeterminate, ...props }: ProgressProps) {
  const isIndeterminate = indeterminate || value == null
  const clampedValue = typeof value === 'number' ? Math.min(100, Math.max(0, value)) : 0

  return (
    <div
      role="progressbar"
      aria-busy={isIndeterminate || undefined}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={isIndeterminate ? undefined : clampedValue}
      className={cn('relative h-1.5 w-full overflow-hidden rounded-full bg-[rgba(0,0,0,0.06)]', className)}
      {...props}
    >
      <div
        className={cn(
          'h-full rounded-full bg-primary transition-all duration-1000',
          isIndeterminate && 'w-1/3 animate-pulse'
        )}
        style={isIndeterminate ? undefined : { width: `${clampedValue}%` }}
      />
    </div>
  )
}

export { Progress }
