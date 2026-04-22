'use client'

import { useEffect, useRef } from 'react'
import { useForm, FormProvider } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useToken, useUpdateToken } from '@/lib/hooks'
import { tokenUpdateSchema, type TokenUpdateInput } from '@/lib/validations'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
  Alert, AlertDescription, AlertTitle, Button, Input, Label, Select, SelectOption, Switch,
} from '@/components/ui'
import { useToast } from '@/components/ui/toaster'
import type { Token } from '@/types'
import { useTranslation } from '@/lib/i18n/context'

interface TokenDialogProps {
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

  return {
    status: token.status,
    pool: token.pool,
    quotas: { ...(token.quotas ?? {}) },
    remark: token.remark ?? '',
    nsfw_enabled: token.nsfw_enabled ?? false,
  }
}

/** Get sorted mode keys from token quotas/limit_quotas */
function getQuotaModes(token?: Token): string[] {
  if (!token) return []
  const keys = new Set([
    ...Object.keys(token.quotas ?? {}),
    ...Object.keys(token.limit_quotas ?? {}),
  ])
  return Array.from(keys).sort()
}

function modeLabel(mode: string, t: ReturnType<typeof useTranslation>['t']): string {
  switch (mode) {
    case 'chat': return t.tokens.chatQuota
    case 'image': return t.tokens.imageQuota
    case 'video': return t.tokens.videoQuota
    default: return mode
  }
}

export function TokenDialog({ open, onOpenChange, tokenId }: TokenDialogProps) {
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

  const modes = getQuotaModes(tokenQuery.data)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
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
                  <Select
                    id="pool"
                    {...form.register('pool')}
                  >
                    <SelectOption value="ssoBasic">{t.dashboard.basicPool}</SelectOption>
                    <SelectOption value="ssoSuper">{t.dashboard.superPool}</SelectOption>
                    <SelectOption value="ssoHeavy">{t.dashboard.heavyPool}</SelectOption>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="status">{t.tokens.status}</Label>
                  <Select
                    id="status"
                    {...form.register('status')}
                  >
                    <SelectOption value="active">{t.tokens.active}</SelectOption>
                    <SelectOption value="disabled">{t.tokens.disabled}</SelectOption>
                    <SelectOption value="expired">{t.tokens.expired}</SelectOption>
                    <SelectOption value="exhausted">{t.tokens.exhausted}</SelectOption>
                  </Select>
                </div>
              </div>

              {modes.length > 0 && (
                <div className="grid gap-4" style={{ gridTemplateColumns: `repeat(${Math.min(modes.length, 3)}, 1fr)` }}>
                  {modes.map((mode) => {
                    const limit = tokenQuery.data?.limit_quotas?.[mode] ?? 0
                    return (
                      <div key={mode} className="space-y-2">
                        <Label htmlFor={`quota-${mode}`}>
                          {modeLabel(mode, t)}
                          <span className="ml-1 text-xs text-muted">/ {limit}</span>
                        </Label>
                        <Input
                          id={`quota-${mode}`}
                          type="number"
                          {...form.register(`quotas.${mode}`, { valueAsNumber: true })}
                          placeholder="0"
                        />
                      </div>
                    )
                  })}
                </div>
              )}

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
