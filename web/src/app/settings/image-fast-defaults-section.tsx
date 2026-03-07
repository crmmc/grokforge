import { Input, Label } from '@/components/ui'
import { ConfigSection } from './config-section'
import type { Dictionary } from '@/lib/i18n/dictionaries'

interface ImageFastDefaultsSectionProps {
  t: Dictionary
  imagineFastN: number
  setImagineFastN: (v: number) => void
  imagineFastSize: string
  setImagineFastSize: (v: string) => void
  setImagineFastDirty: (v: boolean) => void
}

export function ImageFastDefaultsSection({
  t,
  imagineFastN, setImagineFastN,
  imagineFastSize, setImagineFastSize,
  setImagineFastDirty,
}: ImageFastDefaultsSectionProps) {
  return (
    <ConfigSection title={t.config.imagineFastDefaults} description={t.config.imagineFastDefaultsDesc}>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <Label htmlFor="imagine_fast_n">{t.config.imagineFastN}</Label>
          <Input id="imagine_fast_n" type="number" className="max-w-[200px]" min="1" max="4" value={imagineFastN} onChange={(e) => { setImagineFastN(Number(e.target.value)); setImagineFastDirty(true) }} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="imagine_fast_size">{t.config.imagineFastSize}</Label>
          <select id="imagine_fast_size" className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm" value={imagineFastSize} onChange={(e) => { setImagineFastSize(e.target.value); setImagineFastDirty(true) }}>
            <option value="1024x1024">1024x1024</option>
            <option value="1024x1792">1024x1792</option>
            <option value="1792x1024">1792x1024</option>
          </select>
        </div>
      </div>
    </ConfigSection>
  )
}
