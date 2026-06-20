import { Input, Label, Select, SelectOption, Switch } from '@/components/ui'
import { ConfigSection } from './config-section'
import type { Dictionary } from '@/lib/i18n/dictionaries'
import type { UseFormRegister, UseFormWatch, UseFormSetValue } from 'react-hook-form'
import type { ProxyBrowsersResponse } from '@/types'
import type { GeneralInput } from './general-config-form.schema'

interface ProxyConfigSectionProps {
  t: Dictionary
  register: UseFormRegister<GeneralInput>
  watch: UseFormWatch<GeneralInput>
  setValue: UseFormSetValue<GeneralInput>
  proxyEnabled: boolean
  cfAutoRefresh: boolean
  setCfAutoRefresh: (v: boolean) => void
  proxyBrowsers: ProxyBrowsersResponse
}

export function ProxyConfigSection({
  t, register, watch, setValue, proxyEnabled, cfAutoRefresh, setCfAutoRefresh, proxyBrowsers,
}: ProxyConfigSectionProps) {
  const timeoutFieldId = cfAutoRefresh ? 'proxy.timeout.flaresolverr' : 'proxy.timeout.manual'

  return (
    <ConfigSection title={t.config.proxy} description={t.config.proxyDesc}>
      <div className="flex items-center space-x-2">
        <Switch id="proxy.enabled" checked={proxyEnabled} onCheckedChange={(v: boolean) => setValue('proxy.enabled', v, { shouldDirty: true })} />
        <Label htmlFor="proxy.enabled">{t.config.proxyEnabled}</Label>
      </div>
      {proxyEnabled && (
        <>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="proxy.base_proxy_url">{t.config.baseProxyUrl}</Label>
              <Input id="proxy.base_proxy_url" {...register('proxy.base_proxy_url')} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="proxy.asset_proxy_url">{t.config.assetProxyUrl}</Label>
              <Input id="proxy.asset_proxy_url" {...register('proxy.asset_proxy_url')} />
            </div>
          </div>
          <div className="space-y-2">
            <div className="flex items-center space-x-2">
              <Switch
                id="cf_auto_refresh"
                checked={cfAutoRefresh}
                onCheckedChange={(v: boolean) => {
                  setCfAutoRefresh(v)
                  if (v) {
                    setValue('proxy.cf_clearance', '', { shouldDirty: true })
                    setValue('proxy.cf_cookies', '', { shouldDirty: true })
                  } else {
                    setValue('proxy.flaresolverr_url', '', { shouldDirty: true })
                  }
                }}
              />
              <Label htmlFor="cf_auto_refresh">{t.config.cfAutoRefresh}</Label>
            </div>
            <p className="text-sm text-muted">{t.config.cfAutoRefreshDesc}</p>
          </div>
          {cfAutoRefresh ? (
            <div className="grid gap-4 sm:grid-cols-3">
              <div className="space-y-2">
                <Label htmlFor="proxy.flaresolverr_url">{t.config.flaresolverrUrl}</Label>
                <Input id="proxy.flaresolverr_url" {...register('proxy.flaresolverr_url')} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="proxy.refresh_interval">{t.config.proxyRefreshInterval}</Label>
                <Input id="proxy.refresh_interval" type="number" className="max-w-[200px]" min="0" {...register('proxy.refresh_interval', { valueAsNumber: true })} />
                <p className="text-xs text-muted">{t.config.proxyRefreshIntervalDesc}</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor={timeoutFieldId}>{t.config.proxyTimeout}</Label>
                <Input id={timeoutFieldId} type="number" className="max-w-[200px]" min="0" {...register('proxy.timeout', { valueAsNumber: true })} />
              </div>
            </div>
          ) : (
            <>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="proxy.cf_clearance">{t.config.cfClearance}</Label>
                  <Input id="proxy.cf_clearance" {...register('proxy.cf_clearance')} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="proxy.cf_cookies">{t.config.cfCookies}</Label>
                  <Input id="proxy.cf_cookies" {...register('proxy.cf_cookies')} />
                </div>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor={timeoutFieldId}>{t.config.proxyTimeout}</Label>
                  <Input id={timeoutFieldId} type="number" className="max-w-[200px]" min="0" {...register('proxy.timeout', { valueAsNumber: true })} />
                </div>
              </div>
            </>
          )}
          <div className="flex items-center space-x-2">
            <Switch id="proxy.skip_proxy_ssl_verify" checked={watch('proxy.skip_proxy_ssl_verify')} onCheckedChange={(v: boolean) => setValue('proxy.skip_proxy_ssl_verify', v, { shouldDirty: true })} />
            <Label htmlFor="proxy.skip_proxy_ssl_verify">{t.config.skipSslVerify}</Label>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="proxy.browser">{t.config.browser}</Label>
              <Select
                id="proxy.browser"
                disabled={cfAutoRefresh}
                {...register('proxy.browser', {
                  onChange: (e: React.ChangeEvent<HTMLSelectElement>) => {
                    const selected = proxyBrowsers.browsers.find((option) => option.browser === e.target.value)
                    if (selected) {
                      setValue('proxy.user_agent', selected.user_agent, { shouldDirty: true })
                    }
                  },
                })}
              >
                {proxyBrowsers.browsers.map((option) => (
                  <SelectOption key={option.browser} value={option.browser}>{option.label}</SelectOption>
                ))}
              </Select>
              {cfAutoRefresh && <p className="text-xs text-muted">{t.config.managedByFlaresolverr}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="proxy.user_agent">{t.config.userAgent}</Label>
              <Input
                id="proxy.user_agent"
                placeholder={proxyBrowsers.default_user_agent}
                disabled={cfAutoRefresh}
                {...register('proxy.user_agent')}
              />
              {cfAutoRefresh && <p className="text-xs text-muted">{t.config.managedByFlaresolverr}</p>}
            </div>
          </div>
        </>
      )}
    </ConfigSection>
  )
}
