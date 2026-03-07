'use client'

import { ReactNode } from 'react'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui'

interface ConfigSectionProps {
  title: string
  description?: string
  children: ReactNode
}

export function ConfigSection({ title, description, children }: ConfigSectionProps) {
  return (
    <Card>
      <CardHeader className="pb-4">
        <CardTitle className="text-lg">{title}</CardTitle>
        {description && (
          <p className="text-sm text-muted">{description}</p>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        {children}
      </CardContent>
    </Card>
  )
}
