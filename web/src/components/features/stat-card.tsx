'use client'

import { Card, CardContent, CardHeader, CardTitle, Skeleton } from '@/components/ui'
import { TrendingUp, TrendingDown, Minus } from 'lucide-react'

interface StatCardProps {
  title: string
  value: string | number
  subtitle?: React.ReactNode
  icon: React.ElementType
  loading?: boolean
  borderColor?: string
  trend?: number | null
}

export function StatCard({ title, value, subtitle, icon: Icon, loading, borderColor, trend }: StatCardProps) {
  return (
    <Card className={`relative overflow-hidden border-l-4 ${borderColor ?? 'border-l-transparent'}`}>
      <div className="absolute -right-4 -top-4 opacity-5 pointer-events-none">
        <Icon className="h-24 w-24" />
      </div>
      <CardHeader className="flex flex-row items-center justify-between pb-2 relative z-10">
        <CardTitle className="text-sm font-medium text-muted">{title}</CardTitle>
        <Icon className="h-4 w-4 text-muted" />
      </CardHeader>
      <CardContent className="relative z-10">
        {loading ? (
          <Skeleton className="h-8 w-20" />
        ) : (
          <div className="flex flex-col gap-1">
            <div className="flex items-baseline gap-2">
              <span className="text-3xl font-bold tracking-tight">{value}</span>
              {trend != null && <TrendBadge value={trend} />}
            </div>
            {subtitle && (
              <div className="text-xs text-muted font-medium mt-1">
                {subtitle}
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function TrendBadge({ value }: { value: number }) {
  if (value > 0) {
    return (
      <span className="flex items-center gap-0.5 text-xs text-emerald-600">
        <TrendingUp className="h-3 w-3" />
        {Math.abs(value).toFixed(1)}%
      </span>
    )
  }
  if (value < 0) {
    return (
      <span className="flex items-center gap-0.5 text-xs text-rose-600">
        <TrendingDown className="h-3 w-3" />
        {Math.abs(value).toFixed(1)}%
      </span>
    )
  }
  return (
    <span className="flex items-center gap-0.5 text-xs text-muted">
      <Minus className="h-3 w-3" />
      0%
    </span>
  )
}
