'use client'

import { QueryProvider } from '@/lib/query-provider'
import { ToasterProvider } from '@/components/ui'
import { AdminLayout } from './admin-layout'

interface AdminAppShellProps {
  children: React.ReactNode
  noBottomPadding?: boolean
}

export function AdminAppShell({ children, noBottomPadding = false }: AdminAppShellProps) {
  return (
    <QueryProvider>
      <ToasterProvider>
        <AdminLayout noBottomPadding={noBottomPadding}>{children}</AdminLayout>
      </ToasterProvider>
    </QueryProvider>
  )
}
