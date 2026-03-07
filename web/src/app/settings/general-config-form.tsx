'use client'

import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Save, Loader2 } from 'lucide-react'
import { Button, Input, Label, Switch } from '@/components/ui'
import { ConfigSection } from './config-section'
import { SensitiveInput } from './sensitive-input'
import { ProxyConfigSection } from './proxy-config-section'
import { RetryConfigSection } from './retry-config-section'
import { SecurityLimitsSection } from './security-limits-section'
import { generalSchema } from './general-config-form.schema'
import type { ConfigResponse } from '@/types'
import { useTranslation } from '@/lib/i18n/context'
import { useState } from 'react'
import type { GeneralInput } from './general-config-form.schema'

const APP_LOG_LEVELS = ['debug', 'info', 'warn', 'error'] as const
const DB_DRIVERS = ['sqlite', 'postgres'] as const

function normalizeAppConfig(app: ConfigResponse['app']): GeneralInput['app'] {
  const logLevel = APP_LOG_LEVELS.includes(app.log_level as typeof APP_LOG_LEVELS[number])
    ? app.log_level as typeof APP_LOG_LEVELS[number]
    : 'info'

  const dbDriver = DB_DRIVERS.includes(app.db_driver as typeof DB_DRIVERS[number])
    ? app.db_driver as typeof DB_DRIVERS[number]
    : 'sqlite'

  return {
    ...app,
    log_level: logLevel,
    db_driver: dbDriver,
  }
}

interface GeneralConfigFormProps {
  config: ConfigResponse
  onSubmit: (data: Partial<ConfigResponse>) => void
  isPending: boolean
}

export function GeneralConfigForm({ config, onSubmit, isPending }: GeneralConfigFormProps) {
  const { t } = useTranslation()
  const {
    register,
    handleSubmit,
    watch,
    setValue,
    formState: { errors, isDirty },
  } = useForm<GeneralInput>({
    resolver: zodResolver(generalSchema),
    defaultValues: {
      app: normalizeAppConfig(config.app),
      proxy: config.proxy,
      retry: config.retry,
      image: {
        blocked_parallel_enabled: config.image?.blocked_parallel_enabled ?? true,
        blocked_parallel_attempts: config.image?.blocked_parallel_attempts ?? 5,
      },
    },
  })

  const proxyEnabled = watch('proxy.enabled')
  const [cfAutoRefresh, setCfAutoRefresh] = useState(!!config.proxy?.flaresolverr_url?.trim())

  const doSubmit = (data: GeneralInput) => {
    onSubmit({
      app: data.app,
      proxy: data.proxy,
      retry: data.retry,
      image: data.image as Partial<ConfigResponse['image']>,
    } as Partial<ConfigResponse>)
  }

  return (
    <form onSubmit={handleSubmit(doSubmit)} className="space-y-6">
      {/* Server Settings */}
      <ConfigSection title={t.config.server} description={t.config.serverDesc}>
        <p className="text-sm text-muted">{t.config.serverReadOnly}</p>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="app.host">{t.config.host}</Label>
            <Input id="app.host" disabled {...register('app.host')} />
            {errors.app?.host && <p className="text-sm text-destructive">{errors.app.host.message}</p>}
          </div>
          <div className="space-y-2">
            <Label htmlFor="app.port">{t.config.port}</Label>
            <Input id="app.port" type="number" className="max-w-[200px]" disabled {...register('app.port', { valueAsNumber: true })} />
            {errors.app?.port && <p className="text-sm text-destructive">{errors.app.port.message}</p>}
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="app.log_level">{t.config.logLevel}</Label>
            <select id="app.log_level" className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm disabled:opacity-50 disabled:cursor-not-allowed" disabled {...register('app.log_level')}>
              <option value="debug">Debug</option>
              <option value="info">Info</option>
              <option value="warn">Warn</option>
              <option value="error">Error</option>
            </select>
          </div>
          <div className="flex items-center space-x-2 pt-8">
            <Switch id="app.log_json" disabled checked={watch('app.log_json')} onCheckedChange={(v: boolean) => setValue('app.log_json', v, { shouldDirty: true })} />
            <Label htmlFor="app.log_json">{t.config.jsonLogging}</Label>
          </div>
        </div>
      </ConfigSection>

      {/* Authentication */}
      <ConfigSection title={t.config.auth} description={t.config.authDesc}>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="app.app_key">{t.config.adminPassword}</Label>
            <SensitiveInput id="app.app_key" {...register('app.app_key')} />
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="app.request_timeout">{t.config.requestTimeout}</Label>
            <Input id="app.request_timeout" type="number" className="max-w-[200px]" min="1" {...register('app.request_timeout', { valueAsNumber: true })} />
            <p className="text-xs text-muted">{t.config.requestTimeoutDesc}</p>
            {errors.app?.request_timeout && <p className="text-sm text-destructive">{errors.app.request_timeout.message}</p>}
          </div>
        </div>
      </ConfigSection>

      {/* Media Generation Settings */}
      <ConfigSection title={t.config.mediaGeneration} description={t.config.mediaGenerationDesc}>
        <div className="flex items-center space-x-2">
          <Switch id="app.media_generation_enabled" checked={watch('app.media_generation_enabled')} onCheckedChange={(v: boolean) => setValue('app.media_generation_enabled', v, { shouldDirty: true })} />
          <Label htmlFor="app.media_generation_enabled">{t.config.enable}</Label>
        </div>
      </ConfigSection>

      <SecurityLimitsSection t={t} register={register} errors={errors} />

      <ProxyConfigSection
        t={t} register={register} watch={watch} setValue={setValue}
        proxyEnabled={proxyEnabled} cfAutoRefresh={cfAutoRefresh} setCfAutoRefresh={setCfAutoRefresh}
      />

      <RetryConfigSection t={t} register={register} watch={watch} setValue={setValue} />

      {/* Image Blocked Parallel Settings */}
      <ConfigSection title={t.config.imageSettings} description={t.config.imageSettingsDesc}>
        <div className="flex items-center space-x-2">
          <Switch id="image.blocked_parallel_enabled" checked={watch('image.blocked_parallel_enabled')} onCheckedChange={(v: boolean) => setValue('image.blocked_parallel_enabled', v, { shouldDirty: true })} />
          <div>
            <Label htmlFor="image.blocked_parallel_enabled">{t.config.blockedParallelEnabled}</Label>
            <p className="text-xs text-muted">{t.config.blockedParallelEnabledDesc}</p>
          </div>
        </div>
        {watch('image.blocked_parallel_enabled') && (
          <div className="max-w-xs space-y-2">
            <Label htmlFor="image.blocked_parallel_attempts">{t.config.blockedParallelAttempts}</Label>
            <Input id="image.blocked_parallel_attempts" type="number" className="max-w-[200px]" min="1" {...register('image.blocked_parallel_attempts', { valueAsNumber: true })} />
            <p className="text-xs text-muted">{t.config.blockedParallelAttemptsDesc}</p>
          </div>
        )}
      </ConfigSection>

      {/* Submit Button */}
      <div className="sticky bottom-0 z-10 flex justify-end bg-background/95 backdrop-blur-sm py-4 border-t mt-6 -mx-1 px-1">
        <Button type="submit" disabled={!isDirty || isPending} className="shadow-sm">
          {isPending ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
          {t.config.saveChanges}
        </Button>
      </div>
    </form>
  )
}
