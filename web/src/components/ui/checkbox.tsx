import * as React from 'react'
import { cn } from '@/lib/utils'

interface CheckboxProps extends Omit<React.InputHTMLAttributes<HTMLInputElement>, 'onChange'> {
  indeterminate?: boolean
  onCheckedChange?: (checked: boolean) => void
}

const Checkbox = React.forwardRef<HTMLInputElement, CheckboxProps>(
  ({ className, indeterminate, onCheckedChange, ...props }, ref) => {
    const innerRef = React.useRef<HTMLInputElement>(null)

    React.useImperativeHandle(ref, () => innerRef.current!)

    React.useEffect(() => {
      if (innerRef.current) {
        innerRef.current.indeterminate = indeterminate ?? false
      }
    }, [indeterminate])

    return (
      <input
        type="checkbox"
        ref={innerRef}
        className={cn(
          'peer h-4 w-4 shrink-0 rounded-[3px] border border-[rgba(0,0,0,0.35)] shadow-[inset_0_1px_2px_rgba(0,0,0,0.02)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:cursor-not-allowed disabled:opacity-50 checked:bg-primary checked:border-primary checked:text-white accent-primary',
          className
        )}
        onChange={(e) => onCheckedChange?.(e.target.checked)}
        {...props}
      />
    )
  }
)
Checkbox.displayName = 'Checkbox'

export { Checkbox }
