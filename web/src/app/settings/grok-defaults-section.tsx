import { Label, Switch, Textarea } from '@/components/ui'
import { ConfigSection } from './config-section'
import type { Dictionary } from '@/lib/i18n/dictionaries'

interface GrokDefaultsSectionProps {
  t: Dictionary
  grokTemporary: boolean
  setGrokTemporary: (v: boolean) => void
  grokDisableMemory: boolean
  setGrokDisableMemory: (v: boolean) => void
  grokStream: boolean
  setGrokStream: (v: boolean) => void
  grokThinking: boolean
  setGrokThinking: (v: boolean) => void
  grokDynamicStatsig: boolean
  setGrokDynamicStatsig: (v: boolean) => void
  grokCustomInstruction: string
  setGrokCustomInstruction: (v: string) => void
  grokFilterTags: string[]
  setGrokFilterTags: (v: string[]) => void
  setGrokDirty: (v: boolean) => void
}

export function GrokDefaultsSection({
  t,
  grokTemporary, setGrokTemporary,
  grokDisableMemory, setGrokDisableMemory,
  grokStream, setGrokStream,
  grokThinking, setGrokThinking,
  grokDynamicStatsig, setGrokDynamicStatsig,
  grokCustomInstruction, setGrokCustomInstruction,
  grokFilterTags, setGrokFilterTags,
  setGrokDirty,
}: GrokDefaultsSectionProps) {
  return (
    <ConfigSection title={t.config.grokDefaults} description={t.config.grokDefaultsDesc}>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex items-center space-x-2">
          <Switch id="grok_temporary" checked={grokTemporary} onCheckedChange={(v: boolean) => { setGrokTemporary(v); setGrokDirty(true) }} />
          <div>
            <Label htmlFor="grok_temporary">{t.config.temporary}</Label>
            <p className="text-xs text-muted">{t.config.temporaryDesc}</p>
          </div>
        </div>
        <div className="flex items-center space-x-2">
          <Switch id="grok_disable_memory" checked={grokDisableMemory} onCheckedChange={(v: boolean) => { setGrokDisableMemory(v); setGrokDirty(true) }} />
          <div>
            <Label htmlFor="grok_disable_memory">{t.config.disableMemory}</Label>
            <p className="text-xs text-muted">{t.config.disableMemoryDesc}</p>
          </div>
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex items-center space-x-2">
          <Switch id="grok_stream" checked={grokStream} onCheckedChange={(v: boolean) => { setGrokStream(v); setGrokDirty(true) }} />
          <div>
            <Label htmlFor="grok_stream">{t.config.stream}</Label>
            <p className="text-xs text-muted">{t.config.streamDesc}</p>
          </div>
        </div>
        <div className="flex items-center space-x-2">
          <Switch id="grok_thinking" checked={grokThinking} onCheckedChange={(v: boolean) => { setGrokThinking(v); setGrokDirty(true) }} />
          <div>
            <Label htmlFor="grok_thinking">{t.config.thinking}</Label>
            <p className="text-xs text-muted">{t.config.thinkingDesc}</p>
          </div>
        </div>
      </div>
      <div className="flex items-center space-x-2">
        <Switch id="grok_dynamic_statsig" checked={grokDynamicStatsig} onCheckedChange={(v: boolean) => { setGrokDynamicStatsig(v); setGrokDirty(true) }} />
        <div>
          <Label htmlFor="grok_dynamic_statsig">{t.config.dynamicStatsig}</Label>
          <p className="text-xs text-muted">{t.config.dynamicStatsigDesc}</p>
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor="grok_custom_instruction">{t.config.customInstruction}</Label>
        <Textarea id="grok_custom_instruction" rows={3} placeholder="" value={grokCustomInstruction} onChange={(e) => { setGrokCustomInstruction(e.target.value); setGrokDirty(true) }} />
        <p className="text-xs text-muted">{t.config.customInstructionDesc}</p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="grok_filter_tags">{t.config.filterTags}</Label>
        <Textarea
          id="grok_filter_tags"
          rows={3}
          placeholder={"xaiartifact\nxai:tool_usage_card\ngrok:render"}
          value={grokFilterTags.join('\n')}
          onChange={(e) => {
            setGrokFilterTags(e.target.value.split('\n'))
            setGrokDirty(true)
          }}
        />
        <p className="text-xs text-muted">{t.config.filterTagsDesc}</p>
      </div>
    </ConfigSection>
  )
}
