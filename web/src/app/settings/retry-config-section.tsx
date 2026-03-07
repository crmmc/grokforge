import { Input, Label } from '@/components/ui'
import { StatusCodeTagInput } from '@/components/ui/status-code-tag-input'
import { ConfigSection } from './config-section'
import type { Dictionary } from '@/lib/i18n/dictionaries'
import type { UseFormRegister, UseFormWatch, UseFormSetValue } from 'react-hook-form'
import type { GeneralInput } from './general-config-form.schema'

interface RetryConfigSectionProps {
  t: Dictionary
  register: UseFormRegister<GeneralInput>
  watch: UseFormWatch<GeneralInput>
  setValue: UseFormSetValue<GeneralInput>
}

export function RetryConfigSection({ t, register, watch, setValue }: RetryConfigSectionProps) {
  return (
    <ConfigSection title={t.config.retry} description={t.config.retryDesc}>
      <div className="grid gap-4 sm:grid-cols-4">
        <div className="space-y-2">
          <Label htmlFor="retry.max_tokens">{t.config.retryMaxTokens}</Label>
          <Input id="retry.max_tokens" type="number" className="max-w-[200px]" min="1" max="20" {...register('retry.max_tokens', { valueAsNumber: true })} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="retry.per_token_retries">{t.config.perTokenRetries}</Label>
          <Input id="retry.per_token_retries" type="number" className="max-w-[200px]" min="1" max="10" {...register('retry.per_token_retries', { valueAsNumber: true })} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="retry.retry_backoff_base">{t.config.retryBackoffBase}</Label>
          <Input id="retry.retry_backoff_base" type="number" className="max-w-[200px]" min="0" step="0.1" {...register('retry.retry_backoff_base', { valueAsNumber: true })} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="retry.retry_backoff_factor">{t.config.retryBackoffFactor}</Label>
          <Input id="retry.retry_backoff_factor" type="number" className="max-w-[200px]" step="0.1" {...register('retry.retry_backoff_factor', { valueAsNumber: true })} />
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="retry.retry_backoff_max">{t.config.retryBackoffMax}</Label>
          <Input id="retry.retry_backoff_max" type="number" className="max-w-[200px]" min="0" step="0.1" {...register('retry.retry_backoff_max', { valueAsNumber: true })} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="retry.retry_budget">{t.config.retryBudget}</Label>
          <Input id="retry.retry_budget" type="number" className="max-w-[200px]" step="1" min="0" {...register('retry.retry_budget', { valueAsNumber: true })} />
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="retry.reset_session_status_codes">{t.config.resetSessionStatusCodes}</Label>
          <StatusCodeTagInput
            id="retry.reset_session_status_codes"
            codes={watch('retry.reset_session_status_codes') || []}
            onChange={(codes) => setValue('retry.reset_session_status_codes', codes, { shouldDirty: true })}
            placeholder="403"
          />
          <p className="text-xs text-muted">{t.config.resetSessionStatusCodesDesc}</p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="retry.cooling_status_codes">{t.config.coolingStatusCodes}</Label>
          <StatusCodeTagInput
            id="retry.cooling_status_codes"
            codes={watch('retry.cooling_status_codes') || []}
            onChange={(codes) => setValue('retry.cooling_status_codes', codes, { shouldDirty: true })}
            placeholder="429"
          />
          <p className="text-xs text-muted">{t.config.coolingStatusCodesDesc}</p>
        </div>
      </div>
    </ConfigSection>
  )
}
