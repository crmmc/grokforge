import { cn } from '@/lib/utils'
import { Tooltip, TooltipTrigger, TooltipContent } from './tooltip'

type StatusColor = 'active' | 'expired' | 'exhausted' | 'rate_limited' | string

const colorMap: Record<string, { bg: string; text: string; border: string; dot: string }> = {
  active: { bg: 'bg-emerald-500/10', text: 'text-emerald-600 dark:text-emerald-400', border: 'border-emerald-500/20', dot: 'bg-emerald-500' },
  expired: { bg: 'bg-rose-500/10', text: 'text-rose-600 dark:text-rose-400', border: 'border-rose-500/20', dot: 'bg-rose-500' },
  exhausted: { bg: 'bg-amber-500/10', text: 'text-amber-600 dark:text-amber-400', border: 'border-amber-500/20', dot: 'bg-amber-500' },
  rate_limited: { bg: 'bg-amber-500/10', text: 'text-amber-600 dark:text-amber-400', border: 'border-amber-500/20', dot: 'bg-amber-500' },
}

const fallback = { bg: 'bg-zinc-500/10', text: 'text-zinc-600 dark:text-zinc-400', border: 'border-zinc-500/20', dot: 'bg-zinc-400' }

interface StatusBadgeProps {
  status: StatusColor
  label: string
  title?: string
  className?: string
}

export function StatusBadge({ status, label, title, className }: StatusBadgeProps) {
  const colors = colorMap[status] || fallback
  const badge = (
    <div
      className={cn(
        'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium border whitespace-nowrap',
        colors.bg, colors.text, colors.border,
        title && 'cursor-help',
        className
      )}
    >
      <div className={cn('h-1.5 w-1.5 rounded-full', colors.dot)} />
      {label}
    </div>
  )

  if (!title) return badge

  return (
    <Tooltip>
      <TooltipTrigger asChild>{badge}</TooltipTrigger>
      <TooltipContent side="bottom" className="max-w-[300px] break-all">
        {title}
      </TooltipContent>
    </Tooltip>
  )
}
