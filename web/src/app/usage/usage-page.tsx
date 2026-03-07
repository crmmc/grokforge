'use client'

import { useState } from 'react'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'
import { UsageOverview } from './usage-overview'
import { RequestLog } from './request-log'

type Period = 'hour' | 'day' | 'week' | 'month'

export default function UsagePage() {
  const [period, setPeriod] = useState<Period>('day')
  const { t } = useTranslation()

  return (
    <div className="space-y-8 max-w-6xl">
      <div className="flex flex-col gap-1">
        <h1 className="text-3xl font-bold tracking-tight">{t.usage.title}</h1>
        <p className="text-muted text-sm">{t.usage.description}</p>
      </div>

      <Tabs defaultValue="overview">
        <TabsList>
          <TabsTrigger value="overview">{t.usage.overviewTab}</TabsTrigger>
          <TabsTrigger value="logs">{t.usage.requestLogTab}</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-6">
          <Tabs value={period} onValueChange={(v) => setPeriod(v as Period)}>
            <TabsList>
              <TabsTrigger value="hour">{t.usage.periods.hour}</TabsTrigger>
              <TabsTrigger value="day">{t.usage.periods.day}</TabsTrigger>
              <TabsTrigger value="week">{t.usage.periods.week}</TabsTrigger>
              <TabsTrigger value="month">{t.usage.periods.month}</TabsTrigger>
            </TabsList>
            <TabsContent value={period}>
              <UsageOverview period={period} />
            </TabsContent>
          </Tabs>
        </TabsContent>

        <TabsContent value="logs">
          <RequestLog />
        </TabsContent>
      </Tabs>
    </div>
  )
}
