'use client'

import { useEffect, useRef } from 'react'
import { useForm, FormProvider } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { AlertCircle } from 'lucide-react'
import { useToken, useUpdateToken } from '@/lib/hooks'
import type { AdminModelsResponse } from '@/lib/hooks/use-admin-models'
import { buildQuotaCatalogPresentation, buildTokenQuotaGroups, formatQuotaPresentationIssues } from '@/lib/quota-presentation'
import { useQuotaPresentationWarnings } from '@/lib/use-quota-presentation-warnings'
import { tokenUpdateSchema, type TokenUpdateInput } from '@/lib/validations'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
  Alert, AlertDescription, AlertTitle, Button, Input, Label, Select, SelectOption, Switch, Badge,
} from '@/components/ui'
import { useToast } from '@/components/ui/toaster'
import type { Token } from '@/types'
import { useTranslation } from '@/lib/i18n/context'

interface TokenDialogProps {
  catalog?: AdminModelsResponse
  catalogError?: Error | null
  catalogLoading: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenId: number
}

function toFormValues(token?: Token): TokenUpdateInput {
  if (!token) {
    return {
      status: 'active',
      pool: 'ssoBasic',
      quotas: {},
      remark: '',
      nsfw_enabled: false,
    }
  }

  const status = token.status === 'exhausted' ? 'active' : token.status
  return {
    status,
    pool: token.pool,
    quotas: { ...(token.quotas ?? {}) },
    remark: token.remark ?? '',
    nsfw_enabled: token.nsfw_enabled ?? false,
  }
}

export function TokenDialog({
  catalog,
  catalogError,
  catalogLoading,
  open,
  onOpenChange,
  tokenId,
}: TokenDialogProps) {
  const { toast } = useToast()
  const tokenQuery = useToken(open ? tokenId : null)
  const updateToken = useUpdateToken()
  const { t } = useTranslation()
  const hydratedTokenIdRef = useRef<number | null>(null)

  const form = useForm<TokenUpdateInput>({
    resolver: zodResolver(tokenUpdateSchema),
    defaultValues: toFormValues(undefined),
  })

  useEffect(() => {
    if (!open) {
      hydratedTokenIdRef.current = null
      form.reset(toFormValues(undefined))
      return
    }

    if (!tokenQuery.data) {
      return
    }

    if (hydratedTokenIdRef.current === tokenQuery.data.id) {
      return
    }

    form.reset(toFormValues(tokenQuery.data))
    hydratedTokenIdRef.current = tokenQuery.data.id
  }, [form, open, tokenId, tokenQuery.data])

  const selectedPool = form.watch('pool')
  const presentation = catalog ? buildQuotaCatalogPresentation(catalog) : null
  const quotaPresentation = tokenQuery.data && presentation
    ? buildTokenQuotaGroups(selectedPool || tokenQuery.data.pool, tokenQuery.data, presentation)
    : { groups: [], issues: [] }
  const issueMessages = formatQuotaPresentationIssues(quotaPresentation.issues, t)

  useQuotaPresentationWarnings('token-dialog', tokenQuery.data && issueMessages.length > 0
    ? issueMessages.map((message) => `Token #${tokenQuery.data?.id}: ${message}`)
    : catalogError ? [`${t.common.humanReadableQuotaUnavailable}: ${catalogError.message || t.common.unknownError}`] : [])

  const onSubmit = async (data: TokenUpdateInput) => {
    if (!tokenQuery.data) {
      return
    }

    try {
      await updateToken.mutateAsync({ id: tokenQuery.data.id, data })
      toast({
        title: t.tokens.tokenUpdated,
        description: t.tokens.tokenUpdatedDesc,
      })
      onOpenChange(false)
      form.reset()
    } catch {
      toast({ title: t.common.error, description: t.common.operationFailed, variant: 'destructive' })
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[560px]">
        <DialogHeader>
          <DialogTitle>{tokenQuery.data ? `${t.tokens.editToken} #${tokenQuery.data.id}` : t.tokens.editToken}</DialogTitle>
        </DialogHeader>
        {tokenQuery.isLoading ? (
          <div className="py-8 text-center text-sm text-muted">{t.common.loading}</div>
        ) : tokenQuery.isError || !tokenQuery.data ? (
          <>
            <Alert variant="destructive">
              <AlertTitle>{t.common.error}</AlertTitle>
              <AlertDescription>{t.common.operationFailed}</AlertDescription>
            </Alert>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                {t.common.cancel}
              </Button>
            </DialogFooter>
          </>
        ) : (
          <FormProvider {...form}>
            <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="pool">{t.tokens.pool}</Label>
                  <Select id="pool" {...form.register('pool')}>
                    <SelectOption value="ssoBasic">{t.dashboard.basicPool}</SelectOption>
                    <SelectOption value="ssoSuper">{t.dashboard.superPool}</SelectOption>
                    <SelectOption value="ssoHeavy">{t.dashboard.heavyPool}</SelectOption>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="status">{t.tokens.status}</Label>
                  <Select id="status" {...form.register('status')}>
                    <SelectOption value="active">{t.tokens.active}</SelectOption>
                    <SelectOption value="disabled">{t.tokens.disabled}</SelectOption>
                    <SelectOption value="expired">{t.tokens.expired}</SelectOption>
                  </Select>
                </div>
              </div>

              <div className="space-y-3">
                <div>
                  <div className="text-sm font-medium">{t.common.sharedQuota}</div>
                  <div className="text-xs text-muted">{t.common.modelsSharingSameQuota}</div>
                </div>

                {catalogLoading ? (
                  <p className="text-sm text-muted">{t.common.loading}</p>
                ) : catalogError ? (
                  <QuotaAlert title={t.common.humanReadableQuotaUnavailable} message={catalogError.message || t.common.unknownError} />
                ) : quotaPresentation.issues.length > 0 ? (
                  <QuotaAlert title={t.common.humanReadableQuotaUnavailable} message={issueMessages.join(' / ')} />
                ) : null}

                {quotaPresentation.groups.length > 0 ? (
                  <div className="grid gap-4 md:grid-cols-2">
                    {quotaPresentation.groups.map((group) => (
                      <div key={group.key} className="rounded-lg border border-[rgba(0,0,0,0.08)] bg-[rgba(255,255,255,0.92)] p-3">
                        <Label htmlFor={`quota-${group.mode}`} className="text-sm leading-5">
                          {group.title}
                        </Label>
                        <div className="mt-1 text-xs text-muted">
                          {t.common.sharedQuota}: {group.total}
                        </div>
                        <div className="mt-2 flex flex-wrap gap-2">
                          {group.models.map((model) => (
                            <Badge key={model.id} variant="secondary" className="normal-case tracking-normal">
                              {model.displayName}
                            </Badge>
                          ))}
                        </div>
                        <Input
                          id={`quota-${group.mode}`}
                          type="number"
                          className="mt-3"
                          {...form.register(`quotas.${group.mode}`, { valueAsNumber: true })}
                          placeholder="0"
                        />
                      </div>
                    ))}
                  </div>
                ) : !catalogLoading && !catalogError ? (
                  <p className="text-sm text-muted">{t.dashboard.noData}</p>
                ) : null}
              </div>

              <div className="flex items-center justify-between pt-2">
                <Label htmlFor="nsfw_enabled">NSFW {t.tokens.nsfwEnabled}</Label>
                <Switch
                  id="nsfw_enabled"
                  checked={form.watch('nsfw_enabled') ?? false}
                  onCheckedChange={(checked) => form.setValue('nsfw_enabled', checked, { shouldDirty: true })}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="remark">{t.tokens.remark}</Label>
                <Input id="remark" {...form.register('remark')} placeholder={t.tokens.addRemark} maxLength={500} />
                {form.formState.errors.remark && (
                  <p className="text-sm text-destructive">{form.formState.errors.remark.message}</p>
                )}
              </div>

              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                  {t.common.cancel}
                </Button>
                <Button type="submit" disabled={updateToken.isPending}>
                  {t.common.save}
                </Button>
              </DialogFooter>
            </form>
          </FormProvider>
        )}
      </DialogContent>
    </Dialog>
  )
}

function QuotaAlert({ title, message }: { title: string; message: string }) {
  return (
    <Alert variant="warning">
      <AlertCircle className="h-4 w-4" />
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  )
}
