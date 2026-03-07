import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const badgeVariants = cva(
  'inline-flex items-center rounded-[4px] px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wider transition-colors',
  {
    variants: {
      variant: {
        default: 'bg-primary/8 text-primary',
        secondary: 'bg-[rgba(0,0,0,0.05)] text-foreground',
        destructive: 'bg-destructive/8 text-destructive',
        outline: 'border border-[rgba(0,0,0,0.08)] text-foreground',
        success: 'bg-success/10 text-success',
        warning: 'bg-warning/10 text-warning',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
)

export interface BadgeProps extends React.HTMLAttributes<HTMLDivElement>, VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return <div className={cn(badgeVariants({ variant }), className)} {...props} />
}

export { Badge, badgeVariants }
