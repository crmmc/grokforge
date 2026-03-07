'use client'

import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { Save, Loader2 } from 'lucide-react'
import { Button, Input, Label, Switch } from '@/components/ui'
import { ModelTagInput } from '@/components/ui/model-tag-input'
import { ConfigSection } from './config-section'
import { GrokDefaultsSection } from './grok-defaults-section'
import { ImageFastDefaultsSection } from './image-fast-defaults-section'
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
  const [basicModels, setBasicModels] = useState<string[]>(config.token?.basic_models || [])
  const [superModels, setSuperModels] = useState<string[]>(config.token?.super_models || [])
  const [imageNsfw, setImageNsfw] = useState(config.image?.nsfw ?? false)
  const [imageDirty, setImageDirty] = useState(false)
  const [modelsDirty, setModelsDirty] = useState(false)
  const [imagineFastN, setImagineFastN] = useState(config.imagine_fast?.n ?? 1)
  const [imagineFastSize, setImagineFastSize] = useState(config.imagine_fast?.size ?? '1024x1024')
  const [imagineFastDirty, setImagineFastDirty] = useState(false)
  const [grokTemporary, setGrokTemporary] = useState(config.app?.temporary ?? false)
  const [grokDisableMemory, setGrokDisableMemory] = useState(config.app?.disable_memory ?? false)
  const [grokStream, setGrokStream] = useState(config.app?.stream ?? true)
  const [grokThinking, setGrokThinking] = useState(config.app?.thinking ?? false)
  const [grokDynamicStatsig, setGrokDynamicStatsig] = useState(config.app?.dynamic_statsig ?? false)
  const [grokCustomInstruction, setGrokCustomInstruction] = useState(config.app?.custom_instruction ?? '')
  const [grokFilterTags, setGrokFilterTags] = useState<string[]>(config.app?.filter_tags ?? [])
  const [grokDirty, setGrokDirty] = useState(false)
  const {
    register,
    handleSubmit,
    setValue,
    formState: { isDirty },
  } = useForm<TokenInput>({
    resolver: zodResolver(tokenConfigSchema),
    defaultValues: config.token as TokenInput,
  })

  const updateBasicModels = (models: string[]) => {
    setBasicModels(models)
    setValue('basic_models', models, { shouldDirty: true })
    setModelsDirty(true)
  }

  const updateSuperModels = (models: string[]) => {
    setSuperModels(models)
    setValue('super_models', models, { shouldDirty: true })
    setModelsDirty(true)
  }

  const doSubmit = (data: TokenInput) => {
    data.basic_models = basicModels
    data.super_models = superModels
    onSubmit({
      token: data,
      image: { nsfw: imageNsfw } as ConfigResponse['image'],
      imagine_fast: { n: imagineFastN, size: imagineFastSize },
      app: {
        temporary: grokTemporary,
        disable_memory: grokDisableMemory,
        stream: grokStream,
        thinking: grokThinking,
        dynamic_statsig: grokDynamicStatsig,
        custom_instruction: grokCustomInstruction,
        filter_tags: grokFilterTags,
      },
    } as Partial<ConfigResponse>)
  }

  return (
    <form onSubmit={handleSubmit(doSubmit)} className="space-y-6">
      {/* Model Groups */}
      <ConfigSection title={t.config.modelGroups} description={t.config.modelGroupsDesc}>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label>{t.config.basicModels}</Label>
            <ModelTagInput
              id="basic_models"
              models={basicModels}
              onChange={updateBasicModels}
              placeholder={t.config.modelsPlaceholder}
            />
          </div>
          <div className="space-y-2">
            <Label>{t.config.superModels}</Label>
            <ModelTagInput
              id="super_models"
              models={superModels}
              onChange={updateSuperModels}
              placeholder={t.config.modelsPlaceholder}
            />
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="preferred_pool">{t.config.preferredPool}</Label>
            <select id="preferred_pool" className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm" {...register('preferred_pool')}>
              <option value="ssoBasic">{t.dashboard.basicPool}</option>
              <option value="ssoSuper">{t.dashboard.superPool}</option>
            </select>
            <p className="text-sm text-muted">{t.config.preferredPoolDesc}</p>
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor="default_chat_quota">{t.config.defaultChatQuota}</Label>
            <Input id="default_chat_quota" type="number" className="max-w-[200px]" min="0" {...register('default_chat_quota', { valueAsNumber: true })} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="default_image_quota">{t.config.defaultImageQuota}</Label>
            <Input id="default_image_quota" type="number" className="max-w-[200px]" min="0" {...register('default_image_quota', { valueAsNumber: true })} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="default_video_quota">{t.config.defaultVideoQuota}</Label>
            <Input id="default_video_quota" type="number" className="max-w-[200px]" min="0" {...register('default_video_quota', { valueAsNumber: true })} />
          </div>
        </div>
        <p className="text-sm text-muted">{t.config.defaultQuotaDesc}</p>
      </ConfigSection>

      {/* Token Management */}
      <ConfigSection title={t.config.tokenManagement} description={t.config.tokenManagementDesc}>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="fail_threshold">{t.config.failThreshold}</Label>
            <Input id="fail_threshold" type="number" className="max-w-[200px]" min="1" {...register('fail_threshold', { valueAsNumber: true })} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="quota_recovery_mode">{t.config.quotaRecoveryMode}</Label>
            <select id="quota_recovery_mode" className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm" {...register('quota_recovery_mode')}>
              <option value="auto">{t.config.quotaRecoveryAuto}</option>
              <option value="upstream">{t.config.quotaRecoveryUpstream}</option>
            </select>
            <p className="text-xs text-muted">{t.config.quotaRecoveryModeDesc}</p>
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="basic_cool_duration_min">{t.config.basicCoolDuration}</Label>
            <Input id="basic_cool_duration_min" type="number" className="max-w-[200px]" min="0" {...register('basic_cool_duration_min', { valueAsNumber: true })} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="super_cool_duration_min">{t.config.superCoolDuration}</Label>
            <Input id="super_cool_duration_min" type="number" className="max-w-[200px]" min="0" {...register('super_cool_duration_min', { valueAsNumber: true })} />
          </div>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="usage_flush_interval_sec">{t.config.usageFlushInterval}</Label>
            <Input id="usage_flush_interval_sec" type="number" className="max-w-[200px]" min="1" {...register('usage_flush_interval_sec', { valueAsNumber: true })} />
            <p className="text-xs text-muted">{t.config.usageFlushIntervalDesc}</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="cool_check_interval_sec">{t.config.coolCheckInterval}</Label>
            <Input id="cool_check_interval_sec" type="number" className="max-w-[200px]" min="1" {...register('cool_check_interval_sec', { valueAsNumber: true })} />
            <p className="text-xs text-muted">{t.config.coolCheckIntervalDesc}</p>
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

      <ImageFastDefaultsSection
        t={t}
        imagineFastN={imagineFastN} setImagineFastN={setImagineFastN}
        imagineFastSize={imagineFastSize} setImagineFastSize={setImagineFastSize}
        setImagineFastDirty={setImagineFastDirty}
      />

      {/* Selection Algorithm */}
      <ConfigSection title={t.config.selectionAlgorithm} description={t.config.selectionAlgorithmDesc}>
        <div className="max-w-xs space-y-2">
          <Label htmlFor="selection_algorithm">{t.config.selectionAlgorithm}</Label>
          <select id="selection_algorithm" className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm" {...register('selection_algorithm')}>
            <option value="high_quota_first">{t.config.algorithmHighQuota}</option>
            <option value="random">{t.config.algorithmRandom}</option>
            <option value="round_robin">{t.config.algorithmRoundRobin}</option>
          </select>
        </div>
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

      {/* Submit Button */}
      <div className="sticky bottom-0 z-10 flex justify-end bg-background/95 backdrop-blur-sm py-4 border-t mt-6 -mx-1 px-1">
        <Button type="submit" disabled={(!isDirty && !imageDirty && !modelsDirty && !imagineFastDirty && !grokDirty) || isPending} className="shadow-sm">
          {isPending ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
          {t.config.saveChanges}
        </Button>
      </div>
    </form>
  )
}
