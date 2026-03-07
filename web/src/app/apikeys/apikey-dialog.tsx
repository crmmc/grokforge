'use client'

import { useEffect, useState } from 'react'
import { useForm, FormProvider } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useCreateAPIKey, useUpdateAPIKey } from '@/lib/hooks'
import { apiKeyCreateSchema, type APIKeyCreateInput } from '@/lib/validations'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
  Button, Input, Label,
} from '@/components/ui'
import { useToast } from '@/components/ui/toaster'
import { Copy, Check } from 'lucide-react'
import type { APIKey } from '@/types'
import { useTranslation } from '@/lib/i18n/context'

interface APIKeyDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'edit'
  apiKey?: APIKey
}

export function APIKeyDialog({ open, onOpenChange, mode, apiKey }: APIKeyDialogProps) {
  const { toast } = useToast()
  const { t } = useTranslation()
  const createKey = useCreateAPIKey()
  const updateKey = useUpdateAPIKey()
  const [newKey, setNewKey] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  const form = useForm<APIKeyCreateInput>({
    resolver: zodResolver(apiKeyCreateSchema),
    defaultValues: {
      name: apiKey?.name ?? '',
      rate_limit: apiKey?.rate_limit ?? 0,
      daily_limit: apiKey?.daily_limit ?? 0,
    },
  })

  useEffect(() => {
    if (!open) return

    form.reset({
      name: apiKey?.name ?? '',
      rate_limit: apiKey?.rate_limit ?? 0,
      daily_limit: apiKey?.daily_limit ?? 0,
    })
    setNewKey(null)
    setCopied(false)
  }, [form, open, mode, apiKey?.id, apiKey?.name, apiKey?.rate_limit, apiKey?.daily_limit])

  const onSubmit = async (data: APIKeyCreateInput) => {
    try {
      if (mode === 'create') {
        const result = await createKey.mutateAsync(data)
        setNewKey(result.key)
        toast({ title: t.apiKeys.createdTitle, description: t.apiKeys.copyWarning })
      } else if (apiKey) {
        await updateKey.mutateAsync({ id: apiKey.id, data })
        toast({ title: t.common.success, description: t.apiKeys.keyUpdated })
        handleClose()
      }
    } catch {
      toast({ title: t.common.error, description: t.common.operationFailed, variant: 'destructive' })
    }
  }

  const handleClose = () => {
    onOpenChange(false)
    setNewKey(null)
    setCopied(false)
    form.reset({
      name: apiKey?.name ?? '',
      rate_limit: apiKey?.rate_limit ?? 0,
      daily_limit: apiKey?.daily_limit ?? 0,
    })
  }

  const copyKey = async () => {
    if (!newKey) return

    try {
      await navigator.clipboard.writeText(newKey)
      setCopied(true)
      toast({ title: t.common.copied, description: t.apiKeys.copiedToClipboard })
    } catch {
      toast({ title: t.common.error, description: t.common.operationFailed, variant: 'destructive' })
    }
  }

  if (newKey) {
    return (
      <Dialog open={open} onOpenChange={handleClose}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t.apiKeys.createdTitle}</DialogTitle>
          </DialogHeader>
          <div className="space-y-4">
            <p className="text-sm text-muted">
              {t.apiKeys.copyWarning}
            </p>
            <div className="flex items-center gap-2 rounded-md border bg-[rgba(0,0,0,0.04)] p-3">
              <code className="flex-1 break-all text-sm">{newKey}</code>
              <Button variant="ghost" size="icon" onClick={() => void copyKey()} aria-label={t.common.copy}>
                {copied ? <Check className="h-4 w-4 text-green-500" /> : <Copy className="h-4 w-4" />}
              </Button>
            </div>
          </div>
          <DialogFooter>
            <Button onClick={handleClose}>{t.common.done}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{mode === 'create' ? t.apiKeys.createTitle : t.apiKeys.editTitle}</DialogTitle>
        </DialogHeader>
        <FormProvider {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="name">{t.apiKeys.name}</Label>
              <Input id="name" {...form.register('name')} placeholder={t.apiKeys.myApiKey} />
              {form.formState.errors.name && (
                <p className="text-sm text-destructive">{form.formState.errors.name.message}</p>
              )}
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="rate_limit">{t.apiKeys.rateLimit}</Label>
                <Input
                  id="rate_limit"
                  type="number"
                  {...form.register('rate_limit', { valueAsNumber: true })}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="daily_limit">{t.apiKeys.dailyLimit}</Label>
                <Input
                  id="daily_limit"
                  type="number"
                  {...form.register('daily_limit', { valueAsNumber: true })}
                />
              </div>
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={handleClose}>
                {t.common.cancel}
              </Button>
              <Button type="submit" disabled={createKey.isPending || updateKey.isPending}>
                {mode === 'create' ? t.common.create : t.common.save}
              </Button>
            </DialogFooter>
          </form>
        </FormProvider>
      </DialogContent>
    </Dialog>
  )
}
