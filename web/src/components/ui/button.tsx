import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap text-[13px] font-medium transition-all duration-150 ease-out select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0 active:scale-[0.98]',
  {
    variants: {
      variant: {
        default: 'btn-fluent-primary',
        destructive: 'bg-destructive text-white border border-transparent border-b-[rgba(0,0,0,0.2)] shadow-[0_1px_2px_rgba(0,0,0,0.05)] rounded-[4px] hover:bg-[#A80000] active:bg-[#8B0000]',
        outline: 'btn-fluent',
        secondary: 'btn-fluent',
        ghost: 'hover:bg-[rgba(0,0,0,0.03)] rounded-[4px]',
        link: 'text-primary underline-offset-4 hover:underline',
      },
      size: {
        default: 'h-8 px-4 py-1.5 rounded-[4px]',
        sm: 'h-7 px-3 text-[12px] rounded-[4px]',
        lg: 'h-9 px-6 rounded-[4px]',
        icon: 'h-8 w-8 rounded-[4px]',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  }
)

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
  VariantProps<typeof buttonVariants> {}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => {
    return <button className={cn(buttonVariants({ variant, size, className }))} ref={ref} {...props} />
  }
)
Button.displayName = 'Button'

export { Button, buttonVariants }
