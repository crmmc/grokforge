'use client'

import { cn } from '@/lib/utils'
import { useTranslation } from '@/lib/i18n/context'

interface TokenFilterTabsProps {
  activeTab: string
  onTabChange: (tab: string) => void
}

export function TokenFilterTabs({ activeTab, onTabChange }: TokenFilterTabsProps) {
  const { t } = useTranslation()

  const statusTabs = [
    { key: 'all', label: t.tokens.filterAll },
    { key: 'active', label: t.tokens.filterActive, color: 'bg-emerald-50 text-emerald-700 border-emerald-200' },
    { key: 'cooling', label: t.tokens.filterCooling, color: 'bg-amber-50 text-amber-700 border-amber-200' },
    { key: 'expired', label: t.tokens.filterExpired, color: 'bg-rose-50 text-rose-700 border-rose-200' },
  ] as const

  const nsfwTabs = [
    { key: 'nsfw', label: t.tokens.nsfwOn, color: 'bg-fuchsia-50 text-fuchsia-700 border-fuchsia-200' },
    { key: 'no-nsfw', label: t.tokens.nsfwOff, color: 'bg-slate-100 text-slate-700 border-slate-200' },
  ] as const

  return (
    <div
      className="flex items-center gap-1.5 bg-[rgba(0,0,0,0.04)] p-1.5 rounded-full overflow-x-auto"
      role="tablist"
      aria-label="Token status filter"
      style={{ scrollbarWidth: 'none' }}
    >
      {statusTabs.map((tab) => (
        <button
          type="button"
          key={tab.key}
          role="tab"
          aria-selected={activeTab === tab.key}
          onClick={() => onTabChange(tab.key)}
          className={cn(
            'inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs font-medium whitespace-nowrap transition-colors',
            activeTab === tab.key
              ? ('color' in tab ? tab.color : 'border-transparent bg-background text-foreground shadow-sm')
              : 'border-transparent text-muted hover:text-foreground hover:bg-[rgba(0,0,0,0.03)]'
          )}
        >
          {tab.label}
        </button>
      ))}
      <span className="w-px h-4 bg-border mx-1 shrink-0" />
      {nsfwTabs.map((tab) => (
        <button
          type="button"
          key={tab.key}
          role="tab"
          aria-selected={activeTab === tab.key}
          onClick={() => onTabChange(tab.key)}
          className={cn(
            'inline-flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs font-medium whitespace-nowrap transition-colors',
            activeTab === tab.key
              ? tab.color
              : 'border-transparent text-muted hover:text-foreground hover:bg-[rgba(0,0,0,0.03)]'
          )}
        >
          {tab.label}
        </button>
      ))}
    </div>
  )
}
