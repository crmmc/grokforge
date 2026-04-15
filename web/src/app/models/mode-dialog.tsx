'use client'

import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useCreateMode, useUpdateMode } from '@/lib/hooks'
import type { ModelMode } from '@/lib/hooks/use-model-families'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter,
  Button, Input, Label, Select, SelectOption, Switch,
} from '@/components/ui'
import { useToast } from '@/components/ui/toaster'
import { useTranslation } from '@/lib/i18n/context'

const modeSchema = z.object({
  mode: z.string().min(1, 'Mode name is required'),
  upstream_model: z.string().min(1, 'Upstream model is required'),
  upstream_mode: z.string().min(1, 'Upstream mode is required'),
  enabled: z.boolean(),
  quota_cost: z.number().int().min(0),
  pool_floor_override: z.string().optional(),
  quota_override: z.string().optional(),
})

type ModeInput = z.infer<typeof modeSchema>

interface ModeDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  familyId: number
  mode?: ModelMode
}

const defaults: ModeInput = {
  mode: '',
  upstream_model: '',
  upstream_mode: '',
  enabled: true,
  quota_cost: 1,
  pool_floor_override: '',
  quota_override: '',
}

export function ModeDialog({ open, onOpenChange, familyId, mode }: ModeDialogProps) {
  const { t } = useTranslation()
  const { toast } = useToast()
  const createMode = useCreateMode()
  const updateMode = useUpdateMode()
  const isEdit = !!mode

  const form = useForm<ModeInput>({
    resolver: zodResolver(modeSchema),
    defaultValues: defaults,
  })

  useEffect(() => {
    if (!open) { form.reset(defaults); return }
    if (mode) {
      form.reset({
        mode: mode.mode,
        upstream_model: mode.upstream_model,
        upstream_mode: mode.upstream_mode,
        enabled: mode.enabled,
        quota_cost: mode.quota_cost,
        pool_floor_override: mode.pool_floor_override || '',
        quota_override: mode.quota_override || '',
      })
    } else {
      form.reset(defaults)
    }
  }, [open, mode, form])

  const onSubmit = async (data: ModeInput) => {
    try {
      const payload = {
        ...data,
        pool_floor_override: data.pool_floor_override || null,
        quota_override: data.quota_override || null,
      }
      if (isEdit && mode) {
        await updateMode.mutateAsync({ id: mode.id, data: payload })
        toast({ title: t.common.success, description: t.models.updateSuccess })
      } else {
        await createMode.mutateAsync({ ...payload, model_id: familyId })
        toast({ title: t.common.success, description: t.models.createSuccess })
      }
      onOpenChange(false)
    } catch {
      toast({ title: t.common.error, description: t.common.operationFailed, variant: 'destructive' })
    }
  }

  const isPending = createMode.isPending || updateMode.isPending

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>{isEdit ? t.models.editMode : t.models.createMode}</DialogTitle>
        </DialogHeader>
        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="mode">Mode *</Label>
            <Input id="mode" {...form.register('mode')} placeholder="default" />
            {form.formState.errors.mode && <p className="text-sm text-destructive">{form.formState.errors.mode.message}</p>}
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="upstream_model">Upstream Model *</Label>
              <Input id="upstream_model" {...form.register('upstream_model')} placeholder="grok-3" />
              {form.formState.errors.upstream_model && <p className="text-sm text-destructive">{form.formState.errors.upstream_model.message}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="upstream_mode">Upstream Mode *</Label>
              <Input id="upstream_mode" {...form.register('upstream_mode')} placeholder="MODEL_MODE_GROK_3" />
              {form.formState.errors.upstream_mode && <p className="text-sm text-destructive">{form.formState.errors.upstream_mode.message}</p>}
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="quota_cost">Quota Cost</Label>
              <Input id="quota_cost" type="number" {...form.register('quota_cost', { valueAsNumber: true })} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="pool_floor_override">Pool Floor Override</Label>
              <Select id="pool_floor_override" {...form.register('pool_floor_override')}>
                <SelectOption value="">—</SelectOption>
                <SelectOption value="basic">Basic</SelectOption>
                <SelectOption value="super">Super</SelectOption>
                <SelectOption value="heavy">Heavy</SelectOption>
              </Select>
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="quota_override">Quota Override</Label>
            <Input id="quota_override" {...form.register('quota_override')} placeholder="Optional" />
          </div>
          <div className="flex items-center justify-between pt-1">
            <Label htmlFor="enabled">{t.common.enabled}</Label>
            <Switch id="enabled" checked={form.watch('enabled')} onCheckedChange={(v) => form.setValue('enabled', v, { shouldDirty: true })} />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>{t.common.cancel}</Button>
            <Button type="submit" disabled={isPending}>{isEdit ? t.models.saveMode : t.models.createMode}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
