import { Input, Label } from '@/components/ui'
import { ConfigSection } from './config-section'
import type { Dictionary } from '@/lib/i18n/dictionaries'
import type { UseFormRegister, FieldErrors } from 'react-hook-form'
import type { GeneralInput } from './general-config-form.schema'

interface SecurityLimitsSectionProps {
  t: Dictionary
  register: UseFormRegister<GeneralInput>
  errors: FieldErrors<GeneralInput>
}

export function SecurityLimitsSection({ t, register, errors }: SecurityLimitsSectionProps) {
  const appErrors = errors.app
  return (
    <ConfigSection title={t.config.securityLimits} description={t.config.securityLimitsDesc}>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="app.read_header_timeout">{t.config.readHeaderTimeout}</Label>
          <Input id="app.read_header_timeout" type="number" className="max-w-[200px]" min="1" {...register('app.read_header_timeout', { valueAsNumber: true })} />
          <p className="text-xs text-muted">{t.config.readHeaderTimeoutDesc}</p>
          {appErrors?.read_header_timeout && <p className="text-sm text-destructive">{appErrors.read_header_timeout.message}</p>}
        </div>
        <div className="space-y-2">
          <Label htmlFor="app.max_header_bytes">{t.config.maxHeaderBytes}</Label>
          <Input id="app.max_header_bytes" type="number" className="max-w-[200px]" min="4096" {...register('app.max_header_bytes', { valueAsNumber: true })} />
          <p className="text-xs text-muted">{t.config.maxHeaderBytesDesc}</p>
          {appErrors?.max_header_bytes && <p className="text-sm text-destructive">{appErrors.max_header_bytes.message}</p>}
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="app.body_limit">{t.config.bodyLimit}</Label>
          <Input id="app.body_limit" type="number" className="max-w-[200px]" min="4096" {...register('app.body_limit', { valueAsNumber: true })} />
          <p className="text-xs text-muted">{t.config.bodyLimitDesc}</p>
          {appErrors?.body_limit && <p className="text-sm text-destructive">{appErrors.body_limit.message}</p>}
        </div>
        <div className="space-y-2">
          <Label htmlFor="app.chat_body_limit">{t.config.chatBodyLimit}</Label>
          <Input id="app.chat_body_limit" type="number" className="max-w-[200px]" min="4096" {...register('app.chat_body_limit', { valueAsNumber: true })} />
          <p className="text-xs text-muted">{t.config.chatBodyLimitDesc}</p>
          {appErrors?.chat_body_limit && <p className="text-sm text-destructive">{appErrors.chat_body_limit.message}</p>}
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="app.admin_max_fails">{t.config.adminMaxFails}</Label>
          <Input id="app.admin_max_fails" type="number" className="max-w-[200px]" min="1" {...register('app.admin_max_fails', { valueAsNumber: true })} />
          <p className="text-xs text-muted">{t.config.adminMaxFailsDesc}</p>
          {appErrors?.admin_max_fails && <p className="text-sm text-destructive">{appErrors.admin_max_fails.message}</p>}
        </div>
        <div className="space-y-2">
          <Label htmlFor="app.admin_window_sec">{t.config.adminWindowSec}</Label>
          <Input id="app.admin_window_sec" type="number" className="max-w-[200px]" min="10" {...register('app.admin_window_sec', { valueAsNumber: true })} />
          <p className="text-xs text-muted">{t.config.adminWindowSecDesc}</p>
          {appErrors?.admin_window_sec && <p className="text-sm text-destructive">{appErrors.admin_window_sec.message}</p>}
        </div>
      </div>
    </ConfigSection>
  )
}
