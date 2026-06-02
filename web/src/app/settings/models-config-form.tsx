'use client'

import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Save, Loader2 } from 'lucide-react'
import { Button, Input, Label, Select, SelectOption, Switch } from '@/components/ui'
import { ConfigSection } from './config-section'
import { GrokDefaultsSection } from './grok-defaults-section'
import { tokenConfigSchema } from '@/lib/validations/config'
import type { ConfigResponse, TokenConfigResponse } from '@/types'
import { useTranslation } from '@/lib/i18n/context'
import { useState } from 'react'

type TokenInput = TokenConfigResponse

interface ModelsConfigFormProps {
  config: ConfigResponse
  onSubmit: (data: Partial<ConfigResponse>) => void
  isPending: boolean
}

export function ModelsConfigForm({ config, onSubmit, isPending }: ModelsConfigFormProps) {
  const { t } = useTranslation()
  const [imageNsfw, setImageNsfw] = useState(config.image?.nsfw ?? false)
  const [imageDirty, setImageDirty] = useState(false)
  const [grokTemporary, setGrokTemporary] = useState(config.app?.temporary ?? false)
  const [grokDisableMemory, setGrokDisableMemory] = useState(config.app?.disable_memory ?? false)
  const [grokStream, setGrokStream] = useState(config.app?.stream ?? true)
  const [grokThinking, setGrokThinking] = useState(config.app?.thinking ?? false)
  const [grokDynamicStatsig, setGrokDynamicStatsig] = useState(config.app?.dynamic_statsig ?? false)
  const [grokCustomInstruction, setGrokCustomInstruction] = useState(config.app?.custom_instruction ?? '')
  const [grokFilterTags, setGrokFilterTags] = useState<string[]>(config.app?.filter_tags ?? [])
  const [grokDirty, setGrokDirty] = useState(false)
  const [consoleEnabled, setConsoleEnabled] = useState(config.console?.enabled ?? false)
  const [consoleWebSearch, setConsoleWebSearch] = useState(config.console?.web_search ?? false)
  const [consoleDirty, setConsoleDirty] = useState(false)
  const {
    register,
    handleSubmit,
    formState: { isDirty },
  } = useForm<TokenInput>({
    resolver: zodResolver(tokenConfigSchema),
    defaultValues: config.token as TokenInput,
  })

  const doSubmit = (data: TokenInput) => {
    onSubmit({
      token: {
        ...data,
      },
      image: { nsfw: imageNsfw } as ConfigResponse['image'],
      app: {
        temporary: grokTemporary,
        disable_memory: grokDisableMemory,
        stream: grokStream,
        thinking: grokThinking,
        dynamic_statsig: grokDynamicStatsig,
        custom_instruction: grokCustomInstruction,
        filter_tags: grokFilterTags,
      },
      console: {
        enabled: consoleEnabled,
        web_search: consoleEnabled ? consoleWebSearch : false,
      },
    } as Partial<ConfigResponse>)
  }

  return (
    <form onSubmit={handleSubmit(doSubmit)} className="space-y-6">
      {/* Token Management */}
      <ConfigSection title={t.config.tokenManagement} description={t.config.tokenManagementDesc}>
        <div className="grid gap-4 sm:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="fail_threshold">{t.config.failThreshold}</Label>
            <Input id="fail_threshold" type="number" className="max-w-[200px]" min="1" {...register('fail_threshold', { valueAsNumber: true })} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="usage_flush_interval_sec">{t.config.usageFlushInterval}</Label>
            <Input id="usage_flush_interval_sec" type="number" className="max-w-[200px]" min="1" {...register('usage_flush_interval_sec', { valueAsNumber: true })} />
            <p className="text-xs text-muted">{t.config.usageFlushIntervalDesc}</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="selection_algorithm">{t.config.selectionAlgorithm}</Label>
            <Select
              id="selection_algorithm"
              className="max-w-[200px]"
              {...register('selection_algorithm')}
            >
              <SelectOption value="high_quota_first">{t.config.algorithmHighQuota}</SelectOption>
              <SelectOption value="random">{t.config.algorithmRandom}</SelectOption>
              <SelectOption value="round_robin">{t.config.algorithmRoundRobin}</SelectOption>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="recent_use_penalty_sec">{t.config.recentUsePenalty}</Label>
            <Input id="recent_use_penalty_sec" type="number" className="max-w-[200px]" min="0" {...register('recent_use_penalty_sec', { valueAsNumber: true })} />
            <p className="text-xs text-muted">{t.config.recentUsePenaltyDesc}</p>
          </div>
        </div>
      </ConfigSection>

      {/* Image Settings */}
      <ConfigSection title={t.config.imageSettings} description={t.config.imageSettingsDesc}>
        <div className="flex items-center space-x-2">
          <Switch id="image_nsfw" checked={imageNsfw} onCheckedChange={(v: boolean) => { setImageNsfw(v); setImageDirty(true) }} />
          <Label htmlFor="image_nsfw">{t.config.imageNsfw}</Label>
        </div>
        <p className="text-sm text-muted">{t.config.imageNsfwDesc}</p>
      </ConfigSection>

      <GrokDefaultsSection
        t={t}
        grokTemporary={grokTemporary} setGrokTemporary={setGrokTemporary}
        grokDisableMemory={grokDisableMemory} setGrokDisableMemory={setGrokDisableMemory}
        grokStream={grokStream} setGrokStream={setGrokStream}
        grokThinking={grokThinking} setGrokThinking={setGrokThinking}
        grokDynamicStatsig={grokDynamicStatsig} setGrokDynamicStatsig={setGrokDynamicStatsig}
        grokCustomInstruction={grokCustomInstruction} setGrokCustomInstruction={setGrokCustomInstruction}
        grokFilterTags={grokFilterTags} setGrokFilterTags={setGrokFilterTags}
        setGrokDirty={setGrokDirty}
      />

      {/* Beta Features */}
      <ConfigSection title={t.config.betaFeatures} description={t.config.betaFeaturesDesc}>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="flex items-center space-x-2">
            <Switch
              id="console_upstream"
              checked={consoleEnabled}
              onCheckedChange={(v: boolean) => {
                setConsoleEnabled(v)
                if (!v) {
                  setConsoleWebSearch(false)
                }
                setConsoleDirty(true)
              }}
            />
            <div>
              <Label htmlFor="console_upstream">{t.config.consoleUpstream}</Label>
              <p className="text-xs text-muted">{t.config.consoleUpstreamDesc}</p>
            </div>
          </div>
          <div className="flex items-center space-x-2">
            <Switch
              id="console_web_search"
              checked={consoleEnabled && consoleWebSearch}
              disabled={!consoleEnabled}
              onCheckedChange={(v: boolean) => {
                setConsoleWebSearch(v)
                setConsoleDirty(true)
              }}
            />
            <div>
              <Label htmlFor="console_web_search">{t.config.consoleWebSearch}</Label>
              <p className="text-xs text-muted">{t.config.consoleWebSearchDesc}</p>
            </div>
          </div>
        </div>
      </ConfigSection>

      {/* Submit Button */}
      <div className="sticky bottom-0 z-10 flex justify-end bg-background/95 backdrop-blur-sm py-4 border-t mt-6 -mx-1 px-1">
        <Button type="submit" disabled={(!isDirty && !imageDirty && !grokDirty && !consoleDirty) || isPending} className="shadow-sm">
          {isPending ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
          {t.config.saveChanges}
        </Button>
      </div>
    </form>
  )
}
