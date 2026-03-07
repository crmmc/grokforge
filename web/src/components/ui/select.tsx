import * as React from 'react'
import { cn } from '@/lib/utils'

const Select = React.forwardRef<HTMLSelectElement, React.SelectHTMLAttributes<HTMLSelectElement>>(
  ({ className, children, ...props }, ref) => (
    <select
      ref={ref}
      className={cn(
        'flex h-8 w-full rounded-[4px] bg-[rgba(255,255,255,0.7)] hover:bg-[rgba(255,255,255,0.9)] border border-[rgba(0,0,0,0.06)] border-b-[rgba(0,0,0,0.15)] px-3 py-1.5 text-[13px] text-foreground shadow-[inset_0_1px_2px_rgba(0,0,0,0.02)] transition-all duration-150 focus:border-b-primary focus:outline-none disabled:cursor-not-allowed disabled:opacity-50',
        className
      )}
      {...props}
    >
      {children}
    </select>
  )
)
Select.displayName = 'Select'

const SelectOption = React.forwardRef<HTMLOptionElement, React.OptionHTMLAttributes<HTMLOptionElement>>(
  ({ className, ...props }, ref) => <option ref={ref} className={cn('', className)} {...props} />
)
SelectOption.displayName = 'SelectOption'

export { Select, SelectOption }
