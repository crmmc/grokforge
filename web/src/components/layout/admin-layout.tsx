'use client'

import { useState } from 'react'
import { Sidebar } from './sidebar'
import { Header } from './header'
import { useAuthGuard } from '@/lib/auth'

interface AdminLayoutProps {
  children: React.ReactNode
  noBottomPadding?: boolean
}

export function AdminLayout({ children, noBottomPadding = false }: AdminLayoutProps) {
  const { isAuthenticated, isLoading } = useAuthGuard()
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false)

  if (isLoading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="text-muted">Loading...</div>
      </div>
    )
  }

  if (!isAuthenticated) {
    return null
  }

  return (
    <div className="h-screen flex flex-col md:flex-row overflow-hidden relative">
      <Sidebar isOpen={isMobileMenuOpen} onClose={() => setIsMobileMenuOpen(false)} />
      <div className="flex-1 flex flex-col h-full overflow-hidden relative">
        <Header onMenuClick={() => setIsMobileMenuOpen(true)} />
        <main className={`flex-1 overflow-auto relative flex flex-col ${noBottomPadding ? 'p-4 pb-0 md:p-8 md:pb-0' : 'p-4 md:p-8'}`}>
          <div className={`max-w-[1400px] mx-auto w-full flex-1 flex flex-col ${noBottomPadding ? 'pb-0' : 'pb-24'}`}>
            {children}
          </div>
        </main>
      </div>
    </div>
  )
}
