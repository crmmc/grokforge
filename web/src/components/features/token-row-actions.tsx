'use client'

import {
  Button, Tooltip, TooltipTrigger, TooltipContent, buttonVariants,
  DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger, DropdownMenuSeparator,
} from '@/components/ui'
import { RefreshCw, Pencil, Trash2, Ban, CircleCheck, MoreHorizontal } from 'lucide-react'
import type { Token } from '@/types'
import { useTranslation } from '@/lib/i18n/context'
import { cn } from '@/lib/utils'

interface TokenRowActionsProps {
  token: Token
  onEdit: (token: Token) => void
  onDelete: (token: Token) => void
  onRefresh: (token: Token) => void
  onToggleStatus: (token: Token) => void
}

export function TokenRowActions({ token, onEdit, onDelete, onRefresh, onToggleStatus }: TokenRowActionsProps) {
  const { t } = useTranslation()

  return (
    <div className="flex items-center gap-1 justify-end min-w-max">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="icon" onClick={() => onEdit(token)} className="h-8 w-8 text-muted hover:text-foreground" aria-label={t.common.edit}>
            <Pencil className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t.common.edit}</TooltipContent>
      </Tooltip>
      <DropdownMenu>
        <DropdownMenuTrigger
          className={cn(buttonVariants({ variant: 'ghost', size: 'icon' }), 'h-8 w-8 text-muted hover:text-foreground')}
          aria-label={t.tokens.actions}
        >
          <MoreHorizontal className="h-4 w-4" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-40">
          <DropdownMenuItem onClick={() => onToggleStatus(token)}>
            {token.status === 'active' ? (
              <><Ban className="mr-2 h-4 w-4" /> {t.tokens.disable}</>
            ) : (
              <><CircleCheck className="mr-2 h-4 w-4" /> {t.tokens.enable}</>
            )}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => onRefresh(token)}>
            <RefreshCw className="mr-2 h-4 w-4" /> {t.common.refresh}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => onDelete(token)} variant="destructive">
            <Trash2 className="mr-2 h-4 w-4" /> {t.common.delete}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
