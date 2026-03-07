'use client'

import { useMemo, useState, useRef, useEffect } from 'react'
import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { cn } from '@/lib/utils'
import { useTranslation } from '@/lib/i18n/context'
import { clearApiKey } from '@/lib/function-api'
import {
  LayoutDashboard,
  Key,
  KeyRound,
  Settings,
  Activity,
  Sparkles,
  Database,
  LogOut,
} from 'lucide-react'

interface SidebarProps {
  isOpen?: boolean
  onClose?: () => void
}

export function Sidebar({ isOpen, onClose }: SidebarProps) {
  const pathname = usePathname()
  const router = useRouter()
  const { t } = useTranslation()
  const [showMenu, setShowMenu] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const btnRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!showMenu) return
    function handleClickOutside(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node) &&
          btnRef.current && !btnRef.current.contains(e.target as Node)) {
        setShowMenu(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [showMenu])

  const handleLogout = async () => {
    await fetch('/admin/logout', { method: 'POST' })
    clearApiKey()
    router.push('/login/')
  }

  const navItems = useMemo(() => {
    return [
      { href: '/', label: t.nav.dashboard, icon: LayoutDashboard },
      { href: '/tokens', label: t.nav.tokens, icon: Key },
      { href: '/apikeys', label: t.nav.apiKeys, icon: KeyRound },
      { href: '/function', label: t.nav.function, icon: Sparkles },
      { href: '/usage', label: t.nav.usage, icon: Activity },
      { href: '/cache', label: t.nav.cache, icon: Database },
      { href: '/settings', label: t.nav.settings, icon: Settings },
    ]
  }, [t])

  return (
    <>
      {/* Mobile Backdrop */}
      {isOpen && (
        <div 
          className="fixed inset-0 z-40 bg-black/20 backdrop-blur-sm md:hidden"
          onClick={onClose}
        />
      )}
      
      <aside 
        className={cn(
          "fixed inset-y-0 left-0 z-50 w-[280px] h-full shrink-0 flex flex-col pt-12 px-3 border-r border-[rgba(0,0,0,0.06)] bg-white/70 backdrop-blur-[40px] saturate-150 transition-transform duration-300 md:relative md:translate-x-0",
          isOpen ? "translate-x-0" : "-translate-x-full"
        )}
      >
        {/* Brand */}
        <div className="flex items-center px-4 mb-8 mt-2">
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 items-center justify-center rounded-[8px] bg-gradient-to-br from-[#005FB8] to-[#0091FF] shadow-sm">
              <Sparkles className="h-4 w-4 text-white" strokeWidth={2} />
            </div>
            <span className="text-[17px] font-bold tracking-tight text-foreground/90">
              Grok<span className="text-[#005FB8]">Forge</span>
            </span>
          </div>
        </div>

        {/* Navigation */}
        <nav className="flex-1 overflow-y-auto px-1 flex flex-col gap-[2px]">
          {navItems.map((item) => {
            const isActive = pathname === item.href || (item.href !== '/' && pathname.startsWith(item.href))
            return (
              <Link
                key={item.href}
                href={item.href}
                onClick={onClose}
                className={cn(
                  'group flex items-center gap-4 px-3 py-[8px] rounded-[4px] text-sm transition-all duration-150 relative select-none',
                  isActive
                    ? 'bg-[rgba(0,0,0,0.05)] font-semibold text-foreground'
                    : 'text-foreground/90 hover:bg-[rgba(0,0,0,0.03)] active:scale-[0.98]'
                )}
              >
                {/* Fluent 2 Active Indicator Pill */}
                {isActive && (
                  <div className="absolute left-0 top-[20%] bottom-[20%] w-[3px] bg-[#005FB8] rounded-full" />
                )}
                <item.icon
                  className={cn(
                    'w-[18px] h-[18px]',
                    isActive ? 'text-[#005FB8]' : 'text-muted group-hover:text-foreground/80'
                  )}
                  strokeWidth={1.5}
                />
                <span>{item.label}</span>
              </Link>
            )
          })}
        </nav>

        {/* User section — click to show logout */}
        <div className="mt-auto px-1 border-t border-[rgba(0,0,0,0.06)] pt-3 pb-2 mx-3 relative">
          <button
            type="button"
            ref={btnRef}
            onClick={() => setShowMenu((prev) => !prev)}
            className="flex items-center gap-3 p-2 rounded-[4px] hover:bg-[rgba(0,0,0,0.03)] transition-colors w-full text-left"
          >
            <div className="w-7 h-7 rounded-full bg-gradient-to-br from-[#005FB8] to-[#0091FF] flex items-center justify-center text-white text-[11px] font-bold shadow-sm">
              A
            </div>
            <span className="text-sm font-medium text-foreground">Admin</span>
          </button>
          {showMenu && (
            <div
              ref={menuRef}
              className="!absolute w-36 fluent-card-static p-1 shadow-md z-50 bg-[rgba(255,255,255,0.95)] bottom-full mb-2 left-0"
            >
              <button
                type="button"
                className="flex w-full items-center gap-2 px-3 py-2 text-sm text-destructive hover:bg-[rgba(196,43,28,0.08)] rounded-[4px] transition-colors"
                onClick={handleLogout}
              >
                <LogOut className="h-4 w-4" />
                {t.header.logout}
              </button>
            </div>
          )}
        </div>
      </aside>
    </>
  )
}
