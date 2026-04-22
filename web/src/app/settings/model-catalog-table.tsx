'use client'

import { Loader2, AlertCircle } from 'lucide-react'
import { useAdminModels, type AdminModelEntry } from '@/lib/hooks/use-admin-models'
import { useTranslation } from '@/lib/i18n/context'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableHeader,
  TableBody,
  TableHead,
  TableRow,
  TableCell,
} from '@/components/ui/table'

const TYPE_VARIANT: Record<string, 'default' | 'secondary' | 'success' | 'warning'> = {
  chat: 'default',
  image: 'success',
  image_ws: 'success',
  image_edit: 'success',
  image_lite: 'success',
  video: 'warning',
}

const POOL_VARIANT: Record<string, 'default' | 'secondary' | 'outline'> = {
  basic: 'secondary',
  super: 'default',
  heavy: 'outline',
}

function TypeBadge({ type }: { type: string }) {
  return <Badge variant={TYPE_VARIANT[type] ?? 'secondary'}>{type}</Badge>
}

function PoolBadge({ pool }: { pool: string }) {
  return <Badge variant={POOL_VARIANT[pool] ?? 'secondary'}>{pool}</Badge>
}

function FlagsBadges({ model, t }: { model: AdminModelEntry; t: ReturnType<typeof useTranslation>['t'] }) {
  const flags: { label: string; variant: 'default' | 'warning' }[] = []
  if (model.force_thinking) {
    flags.push({ label: t.settings.catalogForceThinking, variant: 'warning' })
  }
  if (model.enable_pro) {
    flags.push({ label: t.settings.catalogEnablePro, variant: 'default' })
  }
  if (!model.enabled) {
    flags.push({ label: t.settings.catalogDisabled, variant: 'warning' })
  }
  if (flags.length === 0) return <span className="text-muted">-</span>
  return (
    <div className="flex flex-wrap gap-1">
      {flags.map((f) => (
        <Badge key={f.label} variant={f.variant} className="text-[10px]">
          {f.label}
        </Badge>
      ))}
    </div>
  )
}

export function ModelCatalogTable() {
  const { data: models, isLoading, error } = useAdminModels()
  const { t } = useTranslation()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-48">
        <Loader2 className="h-6 w-6 animate-spin text-muted" />
      </div>
    )
  }

  if (error) {
    return (
      <Alert variant="destructive">
        <AlertCircle className="h-4 w-4" />
        <AlertDescription>
          {t.common.loadFailed}{': '}{error.message}
        </AlertDescription>
      </Alert>
    )
  }

  if (!models || models.length === 0) {
    return (
      <div className="flex items-center justify-center h-48 text-muted text-sm">
        {t.settings.catalogEmpty}
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <p className="text-muted text-sm">{t.settings.catalogDescription}</p>
      <div className="rounded-lg border border-[rgba(0,0,0,0.06)] overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow className="bg-[rgba(0,0,0,0.02)]">
              <TableHead>{t.settings.catalogId}</TableHead>
              <TableHead>{t.settings.catalogDisplayName}</TableHead>
              <TableHead>{t.settings.catalogType}</TableHead>
              <TableHead>{t.settings.catalogPool}</TableHead>
              <TableHead>{t.settings.catalogQuotaMode}</TableHead>
              <TableHead>{t.settings.catalogUpstreamModel}</TableHead>
              <TableHead>{t.settings.catalogUpstreamMode}</TableHead>
              <TableHead>{t.settings.catalogFlags}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {models.map((m) => (
              <TableRow key={m.id}>
                <TableCell className="font-mono text-xs">{m.id}</TableCell>
                <TableCell>{m.display_name}</TableCell>
                <TableCell><TypeBadge type={m.type} /></TableCell>
                <TableCell><PoolBadge pool={m.pool_floor} /></TableCell>
                <TableCell>{m.mode || <span className="text-muted">-</span>}</TableCell>
                <TableCell className="font-mono text-xs">
                  {m.upstream_model || <span className="text-muted">-</span>}
                </TableCell>
                <TableCell className="font-mono text-xs">
                  {m.upstream_mode || <span className="text-muted">-</span>}
                </TableCell>
                <TableCell><FlagsBadges model={m} t={t} /></TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}
