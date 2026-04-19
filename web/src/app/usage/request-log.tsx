'use client'

import { useState, useDeferredValue } from 'react'
import { useUsageLogs } from '@/lib/hooks'
import {
  Card, CardContent, CardHeader, CardTitle,
  Table, TableHeader, TableBody, TableHead, TableRow, TableCell,
  Select, SelectOption, Button, Skeleton, Input, Alert, AlertDescription,
} from '@/components/ui'
import { useTranslation } from '@/lib/i18n/context'
import { formatDateTime, formatNumber } from '@/lib/utils'
import { ArrowUp, ArrowDown, AlertCircle } from 'lucide-react'

type SortState = { column: string; dir: 'asc' | 'desc' }

const PAGE_SIZES = [20, 50, 100]

export function RequestLog() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [sort, setSort] = useState<SortState>({ column: 'time', dir: 'desc' })
  const [model, setModel] = useState('')
  const [period, setPeriod] = useState('day')
  const [status, setStatus] = useState('')
  const [apiKey, setApiKey] = useState('')

  const deferredModel = useDeferredValue(model)
  const deferredStatus = useDeferredValue(status)
  const deferredApiKey = useDeferredValue(apiKey)
  const statusQuery = deferredStatus.length === 3 ? deferredStatus : ''

  const { data, isLoading, error } = useUsageLogs({
    page,
    pageSize,
    sortBy: sort.column,
    sortDir: sort.dir,
    model: deferredModel,
    period,
    status: statusQuery,
    apiKey: deferredApiKey,
  })

  const toggleSort = (column: string) => {
    setSort((prev) =>
      prev.column === column
        ? { column, dir: prev.dir === 'asc' ? 'desc' : 'asc' }
        : { column, dir: 'desc' }
    )
    setPage(1)
  }

  const SortIcon = ({ column }: { column: string }) => {
    if (sort.column !== column) return null
    return sort.dir === 'asc'
      ? <ArrowUp className="inline h-3 w-3 ml-1" />
      : <ArrowDown className="inline h-3 w-3 ml-1" />
  }

  const sortableHeader = (column: string, label: string, align?: string) => (
    <TableHead className={align ?? ''} aria-sort={sort.column === column ? (sort.dir === 'asc' ? 'ascending' : 'descending') : 'none'}>
      <button
        type="button"
        className="inline-flex w-full items-center gap-1 text-left"
        onClick={() => toggleSort(column)}
      >
        {label}<SortIcon column={column} />
      </button>
    </TableHead>
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t.usage.requestLogTab}</CardTitle>
        <div className="flex flex-wrap gap-3 pt-2">
          <Input
            value={model}
            onChange={(e) => { setModel(e.target.value); setPage(1) }}
            placeholder={t.usage.filterModel}
            className="w-40"
          />
          <Input
            value={status}
            onChange={(e) => {
              setStatus(e.target.value.replace(/\D/g, '').slice(0, 3))
              setPage(1)
            }}
            placeholder={t.usage.filterStatus}
            className="w-28"
            inputMode="numeric"
            maxLength={3}
          />
          <Input
            value={apiKey}
            onChange={(e) => { setApiKey(e.target.value); setPage(1) }}
            placeholder={t.usage.filterApiKey}
            className="w-40"
          />
          <Select
            value={period}
            onChange={(e) => { setPeriod(e.target.value); setPage(1) }}
            className="w-32"
          >
            <SelectOption value="hour">{t.usage.periods.hour}</SelectOption>
            <SelectOption value="day">{t.usage.periods.day}</SelectOption>
            <SelectOption value="week">{t.usage.periods.week}</SelectOption>
            <SelectOption value="month">{t.usage.periods.month}</SelectOption>
          </Select>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : error ? (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>{t.common.loadFailed}{': '}{error.message || t.common.unknownError}</AlertDescription>
          </Alert>
        ) : !data?.data?.length ? (
          <p className="text-center text-muted py-12">{t.usage.noLogData}</p>
        ) : (
          <>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    {sortableHeader('time', t.usage.time)}
                    <TableHead>{t.usage.apiKey}</TableHead>
                    {sortableHeader('model', t.usage.model)}
                    {sortableHeader('ttft', t.usage.ttft, 'text-right')}
                    {sortableHeader('duration', t.usage.duration, 'text-right')}
                    {sortableHeader('tokens_input', t.usage.inputTokens, 'text-right')}
                    {sortableHeader('tokens_output', t.usage.outputTokens, 'text-right')}
                    {sortableHeader('cache_tokens', t.usage.cacheTokens, 'text-right')}
                    {sortableHeader('status', t.usage.status)}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.data.map((entry) => (
                    <TableRow key={entry.id}>
                      <TableCell className="whitespace-nowrap text-sm">
                        {formatDateTime(entry.created_at)}
                      </TableCell>
                      <TableCell className="text-sm">{entry.api_key_name || '-'}</TableCell>
                      <TableCell className="text-sm font-medium">{entry.model}</TableCell>
                      <TableCell className="text-right text-sm">{entry.ttft_ms}</TableCell>
                      <TableCell className="text-right text-sm">{entry.duration_ms}</TableCell>
                      <TableCell className="text-right text-sm">
                        <span className="text-muted-foreground" title={t.usage.estimatedTooltip}>~{formatNumber(entry.tokens_input)}</span>
                      </TableCell>
                      <TableCell className="text-right text-sm">
                        <span className="text-muted-foreground" title={t.usage.estimatedTooltip}>~{formatNumber(entry.tokens_output)}</span>
                      </TableCell>
                      <TableCell className="text-right text-sm">{formatNumber(entry.cache_tokens)}</TableCell>
                      <TableCell>
                        <span className={`text-sm font-medium ${entry.status < 400 ? 'text-emerald-600' : 'text-rose-600'}`}>
                          {entry.status}
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            <div className="flex items-center justify-between pt-4">
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted">{t.usage.pageSize}</span>
                <Select
                  value={String(pageSize)}
                  onChange={(e) => { setPageSize(Number(e.target.value)); setPage(1) }}
                  className="w-20"
                >
                  {PAGE_SIZES.map((s) => (
                    <SelectOption key={s} value={String(s)}>{s}</SelectOption>
                  ))}
                </Select>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}
                >
                  {t.usage.previousPage}
                </Button>
                <span className="text-sm text-muted">
                  {t.usage.pageOf.replace('{page}', String(page)).replace('{total}', String(data.total_pages))}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= data.total_pages}
                  onClick={() => setPage((p) => p + 1)}
                >
                  {t.usage.nextPage}
                </Button>
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
