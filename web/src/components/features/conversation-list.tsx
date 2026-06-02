'use client'

import type { Conversation } from '@/lib/chat-store'
import { useTranslation } from '@/lib/i18n/context'
import { useConfirm } from '@/components/ui/confirm-dialog'
import { Button } from '@/components/ui'
import type { Dictionary } from '@/lib/i18n/dictionaries'
import { Plus, Trash2 } from 'lucide-react'

interface Props {
  conversations: Conversation[]
  activeId: string | null
  onSelect: (id: string) => void
  onDelete: (id: string) => void
  onCreate: () => void
}

function relativeDate(ts: number, t: Dictionary): string {
  const now = Date.now()
  const diff = now - ts
  const day = 86400000
  if (diff < day) return t.common.today
  if (diff < day * 2) return t.common.yesterday
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(new Date(ts))
}

export function ConversationList({ conversations, activeId, onSelect, onDelete, onCreate }: Props) {
  const { t } = useTranslation()
  const confirm = useConfirm()

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between px-3 py-2 border-b">
        <span className="text-xs font-medium text-muted">{t.function.conversations}</span>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={onCreate}
          className="h-7 w-7 text-muted hover:text-foreground"
          title={t.function.newConversation}
          aria-label={t.function.newConversation}
        >
          <Plus className="h-3.5 w-3.5" />
        </Button>
      </div>
      <div className="flex-1 overflow-y-auto">
        {conversations.length === 0 ? (
          <p className="text-xs text-muted px-3 py-4 text-center">{t.function.noConversations}</p>
        ) : (
          conversations.map((c) => (
            <div
              key={c.id}
              role="button"
              tabIndex={0}
              aria-pressed={c.id === activeId}
              onClick={() => onSelect(c.id)}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onSelect(c.id) } }}
              className={`w-full text-left px-3 py-2 text-sm group flex items-center gap-1 transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-inset ${
                c.id === activeId ? 'bg-[rgba(0,0,0,0.03)]' : 'hover:bg-[rgba(0,0,0,0.04)]/50'
              }`}
            >
              <div className="flex-1 min-w-0">
                <div className="truncate">{c.title || t.function.untitledConversation}</div>
                <div className="text-[10px] text-muted">{relativeDate(c.updatedAt, t)}</div>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={async (e) => {
                  e.stopPropagation()
                  if (await confirm({ title: t.function.confirmDeleteConversation, variant: 'destructive' })) onDelete(c.id)
                }}
                className="h-7 w-7 opacity-0 group-hover:opacity-100 text-muted hover:bg-destructive/8 hover:text-destructive shrink-0 [&_svg]:size-3"
                title={t.function.deleteConversation}
                aria-label={t.function.deleteConversation}
              >
                <Trash2 className="h-3 w-3" />
              </Button>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
