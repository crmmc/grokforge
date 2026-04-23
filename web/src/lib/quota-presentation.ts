import type { Dictionary } from '@/lib/i18n/dictionaries'
import type { AdminModelsResponse } from '@/lib/hooks/use-admin-models'
import type { ModeQuotaSummary, Token } from '@/types'

const MODEL_PRIORITY: Record<string, number> = {
  chat: 0,
  image: 1,
  image_edit: 2,
  video: 3,
}

export interface QuotaPresentationIssue {
  code: 'unknown_mode' | 'missing_models'
  mode: string
}

export interface QuotaPresentationModel {
  id: string
  displayName: string
  publicType: string
  poolFloor: string
}

export interface QuotaPresentationGroup {
  key: string
  mode: string
  title: string
  models: QuotaPresentationModel[]
  remaining: number
  total: number
  percent: number
}

export interface QuotaPresentationSummary {
  detailTitle: string
  issuesTitle: string
  percent: number
}

interface CatalogModePresentation {
  mode: string
  defaultQuotas: Record<string, number>
  models: QuotaPresentationModel[]
  order: number
}

export interface QuotaCatalogPresentation {
  modes: Map<string, CatalogModePresentation>
}

export interface QuotaPresentationResult {
  groups: QuotaPresentationGroup[]
  issues: QuotaPresentationIssue[]
}

export function buildQuotaCatalogPresentation(catalog: AdminModelsResponse): QuotaCatalogPresentation {
  const modes = new Map<string, CatalogModePresentation>()

  catalog.mode_groups.forEach((group, index) => {
    modes.set(group.mode, {
      mode: group.mode,
      defaultQuotas: { ...(group.default_quotas ?? {}) },
      models: [],
      order: index,
    })
  })

  catalog.models.forEach((model) => {
    if (!model.enabled || !model.quota_sync || !model.mode) {
      return
    }

    const mode = modes.get(model.mode)
    if (!mode) {
      return
    }

    mode.models.push({
      id: model.id,
      displayName: model.display_name,
      publicType: model.public_type,
      poolFloor: model.pool_floor,
    })
  })

  modes.forEach((mode) => {
    mode.models.sort(comparePresentationModels)
  })

  return { modes }
}

export function buildPoolQuotaGroups(
  pool: string,
  modeQuotas: Record<string, ModeQuotaSummary>,
  catalog: QuotaCatalogPresentation,
): QuotaPresentationResult {
  return buildQuotaPresentation(pool, Object.entries(modeQuotas), catalog, (mode, quota) => ({
    remaining: quota.total_remaining,
    total: quota.total_limit,
  }))
}

export function buildTokenQuotaGroups(
  pool: string,
  token: Pick<Token, 'quotas' | 'limit_quotas'>,
  catalog: QuotaCatalogPresentation,
): QuotaPresentationResult {
  const quotas = token.quotas ?? {}
  const limits = token.limit_quotas ?? {}
  const modes = Array.from(new Set([...Object.keys(quotas), ...Object.keys(limits)])).sort()

  return buildQuotaPresentation(pool, modes.map((mode) => [mode, mode] as const), catalog, (mode) => ({
    remaining: quotas[mode] ?? 0,
    total: limits[mode] ?? quotas[mode] ?? 0,
  }))
}

export function summarizeQuotaGroups(
  groups: QuotaPresentationGroup[],
  issues: QuotaPresentationIssue[],
  t: Dictionary,
): QuotaPresentationSummary {
  const visibleGroups = groups.filter((group) => group.total > 0)
  if (visibleGroups.length === 0) {
    return {
      detailTitle: '',
      issuesTitle: formatQuotaPresentationIssues(issues, t).join('\n'),
      percent: 0,
    }
  }

  const percentTotal = visibleGroups.reduce((sum, group) => sum + group.percent, 0)
  return {
    detailTitle: visibleGroups
      .map((group) => `${group.title}: ${group.remaining} / ${group.total} (${group.percent.toFixed(0)}%)`)
      .join('\n'),
    issuesTitle: formatQuotaPresentationIssues(issues, t).join('\n'),
    percent: percentTotal / visibleGroups.length,
  }
}

export function formatQuotaPresentationIssues(issues: QuotaPresentationIssue[], t: Dictionary): string[] {
  return issues.map((issue) => formatQuotaPresentationIssue(issue, t))
}

export function quotaTextColor(percent: number): string {
  if (percent > 50) return 'text-emerald-700'
  if (percent > 20) return 'text-amber-700'
  return 'text-rose-700'
}

export function quotaSurfaceColor(percent: number): string {
  if (percent > 50) return 'border-emerald-200 bg-emerald-50/70'
  if (percent > 20) return 'border-amber-200 bg-amber-50/70'
  return 'border-rose-200 bg-rose-50/70'
}

export function quotaProgressColor(percent: number): string {
  if (percent > 50) return '[&>div]:bg-emerald-500 bg-emerald-100'
  if (percent > 20) return '[&>div]:bg-amber-500 bg-amber-100'
  return '[&>div]:bg-rose-500 bg-rose-100'
}

function buildQuotaPresentation<T>(
  pool: string,
  entries: ReadonlyArray<readonly [string, T]>,
  catalog: QuotaCatalogPresentation,
  readQuota: (mode: string, value: T) => { remaining: number; total: number },
): QuotaPresentationResult {
  const groups: QuotaPresentationGroup[] = []
  const issues: QuotaPresentationIssue[] = []

  entries.forEach(([mode, value]) => {
    const modeCatalog = catalog.modes.get(mode)
    if (!modeCatalog) {
      issues.push({ code: 'unknown_mode', mode })
      return
    }

    if (!supportsModeInPool(modeCatalog, pool)) {
      return
    }

    const models = modeCatalog.models.filter((model) => supportsPoolFloor(model.poolFloor, pool))
    if (models.length === 0) {
      issues.push({ code: 'missing_models', mode })
      return
    }

    const quota = readQuota(mode, value)
    const total = normalizeQuotaTotal(quota.total, quota.remaining)
    if (total <= 0) {
      return
    }

    groups.push({
      key: mode,
      mode,
      title: buildQuotaGroupTitle(models),
      models,
      remaining: Math.max(quota.remaining, 0),
      total,
      percent: quotaPercent(quota.remaining, total),
    })
  })

  groups.sort((left, right) => {
    if (left.percent !== right.percent) {
      return left.percent - right.percent
    }
    return modeOrderFor(catalog, left.mode) - modeOrderFor(catalog, right.mode)
  })

  return { groups, issues }
}

function comparePresentationModels(left: QuotaPresentationModel, right: QuotaPresentationModel): number {
  const priorityDiff = modelPriorityFor(left.publicType) - modelPriorityFor(right.publicType)
  if (priorityDiff !== 0) {
    return priorityDiff
  }
  return left.displayName.localeCompare(right.displayName)
}

function modelPriorityFor(publicType: string): number {
  return MODEL_PRIORITY[publicType] ?? 99
}

function supportsModeInPool(mode: CatalogModePresentation, pool: string): boolean {
  return (mode.defaultQuotas[poolToShort(pool)] ?? 0) > 0
}

function supportsPoolFloor(poolFloor: string, pool: string): boolean {
  return poolLevelFor(poolFloor) <= poolLevelFor(pool)
}

function poolLevelFor(pool: string): number {
  switch (pool) {
    case 'basic':
    case 'ssoBasic':
      return 1
    case 'super':
    case 'ssoSuper':
      return 2
    case 'heavy':
    case 'ssoHeavy':
      return 3
    default:
      return 0
  }
}

function poolToShort(pool: string): string {
  switch (pool) {
    case 'ssoBasic':
      return 'basic'
    case 'ssoSuper':
      return 'super'
    case 'ssoHeavy':
      return 'heavy'
    default:
      return pool
  }
}

function modeOrderFor(catalog: QuotaCatalogPresentation, mode: string): number {
  return catalog.modes.get(mode)?.order ?? Number.MAX_SAFE_INTEGER
}

function buildQuotaGroupTitle(models: QuotaPresentationModel[]): string {
  if (models.length === 0) {
    return ''
  }
  if (models.length === 1) {
    return models[0].displayName
  }

  const [first, second] = models
  const title = `${first.displayName} / ${second.displayName}`
  return models.length > 2 ? `${title} 等共享` : title
}

function normalizeQuotaTotal(total: number, remaining: number): number {
  return Math.max(total, remaining, 0)
}

function quotaPercent(remaining: number, total: number): number {
  if (total <= 0) {
    return 0
  }
  return Math.max(0, Math.min(100, (Math.max(remaining, 0) / total) * 100))
}

function formatQuotaPresentationIssue(issue: QuotaPresentationIssue, t: Dictionary): string {
  switch (issue.code) {
    case 'missing_models':
      return t.common.quotaMissingModels.replace('{mode}', issue.mode)
    case 'unknown_mode':
    default:
      return t.common.quotaUnknownMode.replace('{mode}', issue.mode)
  }
}
