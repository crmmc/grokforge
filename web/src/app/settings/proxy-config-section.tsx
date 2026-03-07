import { Input, Label, Switch } from '@/components/ui'
import { ConfigSection } from './config-section'
import type { Dictionary } from '@/lib/i18n/dictionaries'
import type { UseFormRegister, UseFormWatch, UseFormSetValue } from 'react-hook-form'
import type { GeneralInput } from './general-config-form.schema'

// 浏览器指纹 → User-Agent 映射（必须成对）
export const BROWSER_UA_MAP: Record<string, string> = {
  chrome_133: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36',
  chrome_144: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36',
  chrome_146: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36',
  firefox_135: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:135.0) Gecko/20100101 Firefox/135.0',
  firefox_147: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:147.0) Gecko/20100101 Firefox/147.0',
}

const DEFAULT_UA = BROWSER_UA_MAP['chrome_146']

interface ProxyConfigSectionProps {
  t: Dictionary
  register: UseFormRegister<GeneralInput>
  watch: UseFormWatch<GeneralInput>
  setValue: UseFormSetValue<GeneralInput>
  proxyEnabled: boolean
  cfAutoRefresh: boolean
  setCfAutoRefresh: (v: boolean) => void
}

export function ProxyConfigSection({
  t, register, watch, setValue, proxyEnabled, cfAutoRefresh, setCfAutoRefresh,
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
              <select
                id="proxy.browser"
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm disabled:opacity-50 disabled:cursor-not-allowed"
                disabled={cfAutoRefresh}
                {...register('proxy.browser', {
                  onChange: (e: React.ChangeEvent<HTMLSelectElement>) => {
                    const ua = BROWSER_UA_MAP[e.target.value]
                    if (ua) {
                      setValue('proxy.user_agent', ua, { shouldDirty: true })
                    }
                  },
                })}
              >
                <option value="chrome_133">Chrome 133</option>
                <option value="chrome_144">Chrome 144</option>
                <option value="chrome_146">Chrome 146</option>
                <option value="firefox_135">Firefox 135</option>
                <option value="firefox_147">Firefox 147</option>
              </select>
              {cfAutoRefresh && <p className="text-xs text-muted">{t.config.managedByFlaresolverr}</p>}
            </div>
            <div className="space-y-2">
              <Label htmlFor="proxy.user_agent">{t.config.userAgent}</Label>
              <Input
                id="proxy.user_agent"
                placeholder={DEFAULT_UA}
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
