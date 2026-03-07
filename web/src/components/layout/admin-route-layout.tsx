'use client'

import { usePathname } from 'next/navigation'
import { AdminAppShell } from './admin-app-shell'

export function AdminRouteLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const noBottomPadding = pathname.startsWith('/settings') || pathname.startsWith('/function')

  return <AdminAppShell noBottomPadding={noBottomPadding}>{children}</AdminAppShell>
}
