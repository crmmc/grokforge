'use client'

import { useState, Fragment, Suspense } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import {
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
  Badge, Checkbox, Progress, Skeleton, StatusBadge,
} from '@/components/ui'
import { ChevronDown, ChevronRight } from 'lucide-react'
import type { AdminModelsResponse } from '@/lib/hooks/use-admin-models'
import type { Token, TokenStatus } from '@/types'
import { cn } from '@/lib/utils'
import { useTranslation } from '@/lib/i18n/context'
import { TokenDetails } from './token-details'
import { TokenFilterTabs } from './token-filter-tabs'
import { TokenRowActions } from './token-row-actions'
import { buildQuotaCatalogPresentation, buildTokenQuotaGroups, formatQuotaPresentationIssues, quotaProgressColor, quotaTextColor, summarizeQuotaGroups } from '@/lib/quota-presentation'
import { useQuotaPresentationWarnings } from '@/lib/use-quota-presentation-warnings'

const statusColors: Record<TokenStatus, string> = {
  active: 'bg-green-700',
  expired: 'bg-red-600',
  exhausted: 'bg-yellow-500',
  disabled: 'bg-gray-400',
}

function maskToken(token: string): string {
  if (!token || token.length <= 20) return token || ''
  return `${token.substring(0, 20)}...`
}

function poolLabel(pool: string, t: ReturnType<typeof useTranslation>['t']): string {
  if (pool === 'ssoHeavy') return t.dashboard.heavyPool
  if (pool === 'ssoSuper') return t.dashboard.superPool
  if (pool === 'ssoBasic') return t.dashboard.basicPool
  return pool || t.dashboard.basicPool
}

interface TokenQuotaRow {
  token: Token
  groups: ReturnType<typeof buildTokenQuotaGroups>['groups']
  issues: ReturnType<typeof buildTokenQuotaGroups>['issues']
  summary: ReturnType<typeof summarizeQuotaGroups>
}

export interface TokenTableProps {
  catalog?: AdminModelsResponse
  catalogError?: Error | null
  catalogLoading: boolean
  tokens: Token[]
  selectedIds: Set<number>
  onSelectionChange: (ids: Set<number>) => void
  onEdit: (token: Token) => void
  onDelete: (token: Token) => void
  onRefresh: (token: Token) => void
  onToggleStatus: (token: Token) => void
}

function TokenTableInner({
  catalog,
  catalogError,
  catalogLoading,
  tokens,
  selectedIds,
  onSelectionChange,
  onEdit,
  onDelete,
  onRefresh,
  onToggleStatus,
}: TokenTableProps) {
  const [expandedIds, setExpandedIds] = useState<Set<number>>(new Set())
  const { t } = useTranslation()
  const searchParams = useSearchParams()
  const router = useRouter()
  const presentation = catalog ? buildQuotaCatalogPresentation(catalog) : null
  const tokenRows = buildTokenRows(tokens, presentation, t)
  const warningMessages = tokenRows.flatMap(({ token, issues }) =>
    formatQuotaPresentationIssues(issues, t).map((message) => `Token #${token.id}: ${message}`)
  )

  useQuotaPresentationWarnings('tokens-table', warningMessages)

  const statusFilter = searchParams.get('status') || ''
  const nsfwFilter = searchParams.get('nsfw') || ''
  const activeTab = nsfwFilter === 'true' ? 'nsfw'
    : nsfwFilter === 'false' ? 'no-nsfw'
    : statusFilter || 'all'

  const setActiveTab = (tab: string) => {
    const params = new URLSearchParams()
    if (tab === 'nsfw') params.set('nsfw', 'true')
    else if (tab === 'no-nsfw') params.set('nsfw', 'false')
    else if (tab !== 'all') params.set('status', tab)
    const qs = params.toString()
    router.push(qs ? `?${qs}` : window.location.pathname, { scroll: false })
  }

  const toggleExpand = (id: number) => {
    const next = new Set(expandedIds)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setExpandedIds(next)
  }

  const toggleSelect = (id: number) => {
    const next = new Set(selectedIds)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    onSelectionChange(next)
  }

  const toggleSelectAll = () => {
    if (selectedIds.size === tokens.length) onSelectionChange(new Set())
    else onSelectionChange(new Set(tokens.map((token) => token.id)))
  }

  const allSelected = tokens.length > 0 && selectedIds.size === tokens.length
  const someSelected = selectedIds.size > 0 && selectedIds.size < tokens.length

  return (
    <div className="space-y-4">
      <TokenFilterTabs activeTab={activeTab} onTabChange={setActiveTab} />

      <div className="rounded-md border border-[rgba(0,0,0,0.06)] shadow-sm bg-card">
        {tokens.length === 0 ? (
          <div className="p-8 text-center text-muted">{t.tokens.noTokens}</div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[40px]">
                  <Checkbox checked={allSelected} indeterminate={someSelected} onCheckedChange={toggleSelectAll} aria-label="Select all" />
                </TableHead>
                <TableHead className="w-[40px]"><span className="sr-only">{t.tokens.expand}</span></TableHead>
                <TableHead className="whitespace-nowrap min-w-[140px]">{t.tokens.tokenHeader}</TableHead>
                <TableHead className="whitespace-nowrap min-w-[100px]">{t.tokens.remark}</TableHead>
                <TableHead className="whitespace-nowrap min-w-[80px]">{t.tokens.pool}</TableHead>
                <TableHead className="whitespace-nowrap min-w-[100px]">{t.tokens.status}</TableHead>
                <TableHead className="whitespace-nowrap min-w-[220px]">{t.common.averageHealth}</TableHead>
                <TableHead className="whitespace-nowrap min-w-[80px]">{t.tokens.nsfw}</TableHead>
                <TableHead className="w-[120px] text-right whitespace-nowrap">{t.tokens.actions}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokenRows.map((row) => {
                const { token, groups, issues, summary } = row
                const isExpanded = expandedIds.has(token.id)
                const isSelected = selectedIds.has(token.id)

                return (
                  <Fragment key={token.id}>
                    <TableRow className={cn(isSelected && 'bg-[rgba(0,0,0,0.04)]/50')}>
                      <TableCell>
                        <Checkbox checked={isSelected} onCheckedChange={() => toggleSelect(token.id)} aria-label={`Select token ${token.id}`} />
                      </TableCell>
                      <TableCell>
                        <button
                          type="button"
                          onClick={() => toggleExpand(token.id)}
                          className="p-1 hover:bg-[rgba(0,0,0,0.04)] rounded"
                          aria-label={isExpanded ? t.common.collapse : t.common.expand}
                        >
                          {isExpanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                        </button>
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-2">
                          <div className={`w-2 h-2 rounded-full ${statusColors[(token.display_status || token.status) as TokenStatus]}`} title={token.display_status || token.status} />
                          <button type="button" onClick={() => onEdit(token)} className="rounded bg-[rgba(0,0,0,0.04)] px-2 py-0.5 text-left text-sm font-mono hover:bg-[rgba(0,0,0,0.04)]/80 min-w-[180px]" title={t.common.edit}>
                            {maskToken(token.token)}
                          </button>
                        </div>
                      </TableCell>
                      <TableCell className="text-sm text-muted" title={token.remark || ''}>
                        {token.remark ? (token.remark.length > 15 ? token.remark.slice(0, 15) + '...' : token.remark) : '-'}
                      </TableCell>
                      <TableCell className="font-medium">{poolLabel(token.pool, t)}</TableCell>
                      <TableCell>
                        <StatusBadge
                          status={token.display_status || token.status}
                          label={statusLabel(token.display_status || token.status, t)}
                          title={token.status_reason || ''}
                        />
                      </TableCell>
                      <TableCell>
                        {catalogLoading ? (
                          <div className="flex min-w-[220px] items-center gap-3">
                            <Skeleton className="h-2 flex-1" />
                            <Skeleton className="h-4 w-12" />
                          </div>
                        ) : catalogError ? (
                          <Badge variant="warning">{t.common.mappingIssue}</Badge>
                        ) : (
                          <div className="flex min-w-[220px] items-center gap-3" title={buildSummaryTitle(summary)}>
                            <Progress
                              value={summary.percent}
                              className={cn('h-2 flex-1', quotaProgressColor(summary.percent))}
                            />
                            <span className={cn('w-12 text-right text-xs font-semibold tabular-nums', quotaTextColor(summary.percent))}>
                              {summary.percent.toFixed(0)}%
                            </span>
                            {issues.length > 0 && <Badge variant="warning">{t.common.mappingIssue}</Badge>}
                          </div>
                        )}
                      </TableCell>
                      <TableCell className="whitespace-nowrap">
                        <Badge variant={token.nsfw_enabled ? 'destructive' : 'secondary'} className="whitespace-nowrap">
                          {token.nsfw_enabled ? t.common.on : t.common.off}
                        </Badge>
                      </TableCell>
                      <TableCell className="w-[120px] whitespace-nowrap">
                        <TokenRowActions token={token} onEdit={onEdit} onDelete={onDelete} onRefresh={onRefresh} onToggleStatus={onToggleStatus} />
                      </TableCell>
                    </TableRow>
                    {isExpanded && (
                      <TableRow key={`${token.id}-details`}>
                        <TableCell colSpan={9} className="bg-[rgba(0,0,0,0.02)] p-4">
                          <TokenDetails
                            catalogError={catalogError}
                            catalogLoading={catalogLoading}
                            groups={groups}
                            issues={issues}
                            token={token}
                          />
                        </TableCell>
                      </TableRow>
                    )}
                  </Fragment>
                )
              })}
            </TableBody>
          </Table>
        )}
      </div>
    </div>
  )
}

export function TokenTable(props: TokenTableProps) {
  return (
    <Suspense fallback={<div className="animate-pulse bg-[rgba(0,0,0,0.04)] h-64 rounded" />}>
      <TokenTableInner {...props} />
    </Suspense>
  )
}

function buildTokenRows(
  tokens: Token[],
  presentation: ReturnType<typeof buildQuotaCatalogPresentation> | null,
  t: ReturnType<typeof useTranslation>['t'],
): TokenQuotaRow[] {
  if (!presentation) {
    return tokens.map((token) => ({
      token,
      groups: [],
      issues: [],
      summary: summarizeQuotaGroups([], [], t),
    }))
  }

  return tokens.map((token) => {
    const { groups, issues } = buildTokenQuotaGroups(token.pool, token, presentation)
    return {
      token,
      groups,
      issues,
      summary: summarizeQuotaGroups(groups, issues, t),
    }
  })
}

function buildSummaryTitle(summary: ReturnType<typeof summarizeQuotaGroups>): string {
  return [summary.detailTitle, summary.issuesTitle].filter(Boolean).join('\n')
}

function statusLabel(status: string, t: ReturnType<typeof useTranslation>['t']): string {
  if (status === 'active') return t.tokens.active
  if (status === 'expired') return t.tokens.expired
  if (status === 'exhausted') return t.tokens.exhausted
  return t.tokens.disabled
}
