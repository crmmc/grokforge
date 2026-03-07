'use client'

import { useMemo } from 'react'
import { Card, CardContent, CardHeader, CardTitle, Skeleton } from '@/components/ui'
import { AreaChart, Area, XAxis, YAxis, Tooltip, Legend, ResponsiveContainer } from 'recharts'
import { BarChart3 } from 'lucide-react'
import type { HourlyUsage } from '@/types'

interface UsageChartProps {
  title: string
  hourly: HourlyUsage[] | undefined
  loading: boolean
  labels: { chat: string; image: string; video: string; noData: string }
}

export function UsageChart({ title, hourly, loading, labels }: UsageChartProps) {
  const chartData = useMemo(() => {
    if (!hourly?.length) return []
    const map = new Map<string, { hour: string; chat: number; image: number; video: number }>()
    for (let i = 0; i < 24; i++) {
      const h = String(i).padStart(2, '0')
      map.set(h, { hour: h, chat: 0, image: 0, video: 0 })
    }
    for (const item of hourly) {
      const entry = map.get(item.hour)
      if (entry && (item.endpoint === 'chat' || item.endpoint === 'image' || item.endpoint === 'video')) {
        entry[item.endpoint] = item.count
      }
    }
    return Array.from(map.values())
  }, [hourly])

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className="h-[300px] w-full" />
        ) : chartData.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-[300px] border-2 border-dashed border-border/60 rounded-lg text-muted">
            <BarChart3 className="h-12 w-12 text-muted/30 mb-3" />
            <p className="text-sm font-medium">{labels.noData}</p>
          </div>
        ) : (
          <ResponsiveContainer width="100%" height={300}>
            <AreaChart data={chartData} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
              <XAxis dataKey="hour" tick={{ fontSize: 12 }} tickMargin={8} />
              <YAxis tick={{ fontSize: 12 }} tickMargin={8} />
              <Tooltip />
              <Legend />
              <Area type="monotone" dataKey="chat" name={labels.chat} stackId="1" stroke="#3b82f6" fill="#3b82f6" fillOpacity={0.6} />
              <Area type="monotone" dataKey="image" name={labels.image} stackId="1" stroke="#10b981" fill="#10b981" fillOpacity={0.6} />
              <Area type="monotone" dataKey="video" name={labels.video} stackId="1" stroke="#f59e0b" fill="#f59e0b" fillOpacity={0.6} />
            </AreaChart>
          </ResponsiveContainer>
        )}
      </CardContent>
    </Card>
  )
}
