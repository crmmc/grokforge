import * as React from 'react'
import { cn } from '@/lib/utils'

const Textarea = React.forwardRef<HTMLTextAreaElement, React.TextareaHTMLAttributes<HTMLTextAreaElement>>(
  ({ className, ...props }, ref) => {
    return (
      <textarea
        className={cn(
          'flex min-h-[60px] w-full rounded-[4px] bg-[rgba(255,255,255,0.7)] hover:bg-[rgba(255,255,255,0.9)] focus:bg-white border border-[rgba(0,0,0,0.06)] border-b-[rgba(0,0,0,0.15)] px-3 py-2 text-[13px] text-foreground shadow-[inset_0_1px_2px_rgba(0,0,0,0.02)] transition-all duration-150 placeholder:text-muted/80 focus:border-b-primary focus:outline-none disabled:cursor-not-allowed disabled:opacity-50',
          className
        )}
        ref={ref}
        {...props}
      />
    )
  }
)
Textarea.displayName = 'Textarea'

export { Textarea }
